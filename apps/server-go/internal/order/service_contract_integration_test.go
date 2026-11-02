package order

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/platform/auth"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"strings"
	"testing"
	"time"
)

func contractDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORDER_TEST_DSN")
	if dsn == "" {
		t.Skip("ORDER_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	var name string
	if e = db.QueryRow(`SELECT current_database()`).Scan(&name); e != nil || !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable test database required", e)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func TestServiceContractsAcrossFourServices(t *testing.T) {
	db := contractDB(t)
	ctx := context.Background()
	for _, sample := range catalog.Examples() {
		t.Run(sample.Key, func(t *testing.T) {
			stamp := fmt.Sprint(time.Now().UnixNano())
			var customer, cover, evidence int64
			must := func(e error) {
				t.Helper()
				if e != nil {
					t.Fatal(e)
				}
			}
			must(db.QueryRow(`INSERT INTO customer(org_id,display_name,source_channel,mobile_cipher,mobile_hash) VALUES(1,$1::text,'TEST',decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, stamp).Scan(&customer))
			must(db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'ADMIN',1,'SKU_IMAGE','IMAGE','cover.png',$1,'image/png',1,'READY') RETURNING id`, "cover-"+stamp).Scan(&cover))
			must(db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'CUSTOMER',$1,'DEMAND_EVIDENCE','IMAGE','site.png',$2,'image/png',1,'READY') RETURNING id`, customer, "site-"+stamp).Scan(&evidence))
			p := auth.Principal{OrgID: 1, SubjectID: customer, Role: "CUSTOMER", Name: "测试客户"}
			cat := catalog.New(db)
			carts := cart.New(db)
			orders := New(db)
			w := catalog.SKUWrite{CategoryID: "1001", Name: sample.Name, PriceMode: "FIXED", Unit: sample.Unit, BasePrice: sample.Price, CoverMediaID: fmt.Sprint(cover), DetailProfile: sample.Profile}
			sku, e := cat.Create(ctx, w, "test")
			must(e)
			var sid int64
			fmt.Sscan(sku.ID, &sid)
			t.Cleanup(func() {
				db.Exec(`DELETE FROM order_item_media WHERE order_item_id IN(SELECT id FROM order_item WHERE sku_id=$1)`, sid)
				db.Exec(`DELETE FROM order_status_history WHERE order_id IN(SELECT id FROM customer_order WHERE customer_id=$1)`, customer)
				db.Exec(`DELETE FROM order_item WHERE sku_id=$1`, sid)
				db.Exec(`DELETE FROM customer_order WHERE customer_id=$1`, customer)
				db.Exec(`DELETE FROM idempotency_record WHERE principal_id=$1 AND principal_type='CUSTOMER'`, customer)
				db.Exec(`DELETE FROM shopping_cart_item_media WHERE cart_item_id IN(SELECT id FROM shopping_cart_item WHERE sku_id=$1)`, sid)
				db.Exec(`DELETE FROM shopping_cart_item WHERE sku_id=$1`, sid)
				db.Exec(`DELETE FROM shopping_cart WHERE customer_id=$1`, customer)
				db.Exec(`DELETE FROM sku_snapshot_media WHERE sku_id=$1`, sid)
				db.Exec(`DELETE FROM service_sku_version WHERE sku_id=$1`, sid)
				db.Exec(`DELETE FROM service_sku_media WHERE sku_id=$1`, sid)
				db.Exec(`DELETE FROM service_sku WHERE id=$1`, sid)
				db.Exec(`DELETE FROM media_asset WHERE id IN($1,$2)`, cover, evidence)
				db.Exec(`DELETE FROM customer WHERE id=$1`, customer)
			})
			pub, e := cat.Publish(ctx, sid, "test")
			must(e)
			pubAgain, e := cat.Publish(ctx, sid, "test")
			must(e)
			if pubAgain.PublishedVersion != pub.PublishedVersion {
				t.Fatal("duplicate publish created version")
			}
			n := sample.Profile.Billing.Quantity.Min
			cv, e := carts.Add(ctx, p, sid, n)
			must(e)
			if len(cv.Items) != 1 {
				t.Fatal(cv)
			}
			ci := cv.Items[0]
			_, e = carts.Quantity(ctx, p, mustID(t, ci.ID), sample.Profile.Billing.Quantity.Max+1)
			if e == nil {
				t.Fatal("excess cart quantity accepted")
			}
			direct := Write{ContactName: "测试客户", ContactMobile: "13800138000", ServiceAddress: "汉川市测试路六号", AppointmentDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), AppointmentSlot: "08:00", DirectItem: &DirectItem{SKUID: sku.ID, SKUVersion: pub.PublishedVersion, Quantity: n}}
			if sample.Profile.DemandCollection.Description.Mode == "REQUIRED" {
				direct.DirectItem.DemandDescription = "约定设备或现场需求说明"
			}
			if sample.Profile.DemandCollection.Media.Mode == "REQUIRED" {
				if _, e = orders.Create(ctx, p, "missing-"+stamp, direct); e == nil {
					t.Fatal("required evidence not enforced")
				}
				direct.DirectItem.MediaIDs = []string{fmt.Sprint(evidence)}
			}
			// Invalid media is rejected after item insertion; the entire order/idempotency transaction must roll back.
			bad := direct
			badDirect := *direct.DirectItem
			bad.DirectItem = &badDirect
			bad.DirectItem.MediaIDs = []string{fmt.Sprint(cover)}
			if _, err := orders.Create(ctx, p, "bad-media-"+stamp, bad); err == nil {
				t.Fatal("admin media accepted as customer evidence")
			}
			var badOrders int
			must(db.QueryRow(`SELECT count(*) FROM customer_order WHERE customer_id=$1`, customer).Scan(&badOrders))
			if badOrders != 0 {
				t.Fatal("partial order survived media rejection")
			}
			result, e := orders.Create(ctx, p, "direct-"+stamp, direct)
			must(e)
			if result.TotalAmount != sample.Price*int64(n) {
				t.Fatal("wrong amount")
			}
			after, e := carts.Get(ctx, p)
			must(e)
			if len(after.Items) != 1 {
				t.Fatal("direct order modified cart")
			}
			w.Version = sku.Version
			w.DetailProfile.Billing.FeeDescription = "内容已经更新，请按新版本服务费用约定确认"
			_, e = cat.Update(ctx, sid, w, "test")
			must(e)
			pub2, e := cat.Publish(ctx, sid, "test")
			must(e)
			if pub2.PublishedVersion != 2 {
				t.Fatal(pub2)
			}
			if _, e = orders.Create(ctx, p, "stale-"+stamp, direct); e == nil {
				t.Fatal("stale version accepted")
			}
			view, e := orders.Detail(ctx, mustID(t, result.ID), 1)
			must(e)
			if view.Items[0].DetailProfile.Billing.FeeDescription == w.DetailProfile.Billing.FeeDescription {
				t.Fatal("order snapshot changed")
			}
			customerView, e := fulfillment.New(db, nil).CustomerOrder(ctx, p, mustID(t, result.ID))
			must(e)
			if len(customerView.Items) != 1 || customerView.Items[0].DetailProfile.Billing.UnitDefinition != sample.Profile.Billing.UnitDefinition {
				t.Fatal("customer snapshot unavailable")
			}
			cv, e = carts.Get(ctx, p)
			must(e)
			if !cv.Items[0].NeedsConfirmation {
				t.Fatal("missing content change marker")
			}
			if _, err := carts.Confirm(ctx, p, mustID(t, ci.ID), pub.PublishedVersion); err == nil {
				t.Fatal("confirmation accepted an unreviewed version")
			}
			_, e = carts.Confirm(ctx, p, mustID(t, ci.ID), pub2.PublishedVersion)
			must(e)
			_, e = carts.Demand(ctx, p, mustID(t, ci.ID), cart.DemandWrite{DemandDescription: direct.DirectItem.DemandDescription, MediaIDs: direct.DirectItem.MediaIDs})
			must(e)
			direct.DirectItem = nil
			direct.CartItemIDs = []string{ci.ID}
			_, e = orders.Create(ctx, p, "cart-"+stamp, direct)
			must(e)
			if sample.Key == "DEMO-CLEAN" {
				latest, err := cat.Detail(ctx, sid)
				must(err)
				w.Version = latest.Version
				w.DetailProfile.Billing.Quantity = catalog.QuantityRule{Min: 2, Max: 8, Step: 2, Aggregation: "ADDITIVE"}
				_, err = cat.Update(ctx, sid, w, "test")
				must(err)
				_, err = cat.Publish(ctx, sid, "test")
				must(err)
				merged, err := carts.Add(ctx, p, sid, 2)
				must(err)
				merged, err = carts.Add(ctx, p, sid, 2)
				must(err)
				if merged.Items[0].Quantity != 4 {
					t.Fatal("merge quantity incorrect")
				}
				if _, err = carts.Quantity(ctx, p, mustID(t, merged.Items[0].ID), 3); err == nil {
					t.Fatal("cart accepted invalid step")
				}
				if _, err = carts.Add(ctx, p, sid, 6); err == nil {
					t.Fatal("merged excess quantity accepted")
				}
				check, err := carts.Get(ctx, p)
				must(err)
				if check.Items[0].Quantity != 4 {
					t.Fatal("rejected merge modified cart")
				}
				results := make(chan error, 3)
				for i := 0; i < 3; i++ {
					go func() { _, err := carts.Add(ctx, p, sid, 2); results <- err }()
				}
				success := 0
				for i := 0; i < 3; i++ {
					if <-results == nil {
						success++
					}
				}
				if success != 2 {
					t.Fatalf("concurrent add successes=%d, expected 2", success)
				}
				check, err = carts.Get(ctx, p)
				must(err)
				if check.Items[0].Quantity != 8 {
					t.Fatal("concurrent add exceeded max")
				}

			}

		})
	}
}
func mustID(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	if _, e := fmt.Sscan(s, &n); e != nil {
		t.Fatal(e)
	}
	return n
}

func testServiceProfile() catalog.Profile {
	p := catalog.Examples()[3].Profile
	p.DemandCollection.Description.Mode = "OPTIONAL"
	p.Billing.Quantity.Max = 99
	return p
}
