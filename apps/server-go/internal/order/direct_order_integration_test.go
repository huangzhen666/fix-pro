package order

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/platform/auth"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestDirectOrderAndCartIsolation(t *testing.T) {
	dsn := os.Getenv("ORDER_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable ORDER_TEST_DSN required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var database string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(database, "_test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	var customer int64
	if err = db.QueryRow(`INSERT INTO customer(org_id,display_name,source_channel,mobile_cipher,mobile_hash) VALUES(1,$1::text,'TEST',decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, stamp).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM order_status_history WHERE org_id=1 AND order_id IN (SELECT id FROM customer_order WHERE customer_id=$1)`, customer)
		db.Exec(`DELETE FROM order_item WHERE org_id=1 AND order_id IN (SELECT id FROM customer_order WHERE customer_id=$1)`, customer)
		db.Exec(`DELETE FROM customer_order WHERE customer_id=$1`, customer)
		db.Exec(`DELETE FROM idempotency_record WHERE principal_type='CUSTOMER' AND principal_id=$1`, customer)
		db.Exec(`DELETE FROM shopping_cart_item WHERE cart_id IN (SELECT id FROM shopping_cart WHERE customer_id=$1)`, customer)
		db.Exec(`DELETE FROM shopping_cart WHERE customer_id=$1`, customer)
		db.Exec(`DELETE FROM customer WHERE id=$1`, customer)
	})
	var sku int64
	if err = db.QueryRow(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,base_price,unit,current_published_version,status) VALUES(1,1001,$1,'测试维修','测试范围','无','测试质保','FIXED',11800,'次',1,'PUBLISHED') RETURNING id`, stamp).Scan(&sku); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM service_sku_version WHERE sku_id=$1`, sku)
		db.Exec(`DELETE FROM service_sku WHERE id=$1`, sku)
	})
	snapshot, _ := json.Marshal(catalog.PublishedSKU{ID: fmt.Sprint(sku), SKUCode: stamp, Name: "测试维修", ServiceScope: "测试范围", Exclusions: "无", WarrantyDescription: "测试质保", PriceMode: "FIXED", Price: 11800, Unit: "次", CoverImageURL: "/api/v1/media/1", PublishedVersion: 1})
	if _, err = db.Exec(`INSERT INTO service_sku_version(org_id,sku_id,version_no,snapshot_json,published_by) VALUES(1,$1,1,$2,'TEST')`, sku, snapshot); err != nil {
		t.Fatal(err)
	}
	p := auth.Principal{OrgID: 1, SubjectID: customer, Role: "CUSTOMER", Name: "测试客户"}
	service := New(db)
	write := Write{ContactName: "测试客户", ContactMobile: "13800138000", ServiceAddress: "汉川市测试路6号楼203室", AppointmentDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), AppointmentSlot: "08:00", DirectItem: &DirectItem{SKUID: fmt.Sprint(sku), SKUVersion: 1, Quantity: 2}}
	// A direct order does not require a cart and charges only the requested quantity.
	result, err := service.Create(ctx, p, "direct-no-cart-"+stamp, write)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalAmount != 23600 {
		t.Fatalf("total=%d", result.TotalAmount)
	}
	replay, err := service.Create(ctx, p, "direct-no-cart-"+stamp, write)
	if err != nil || replay.ID != result.ID {
		t.Fatalf("idempotent replay: %+v %v", replay, err)
	}
	carts := cart.New(db)
	before, err := carts.Add(ctx, p, sku, 5)
	if err != nil {
		t.Fatal(err)
	}
	result, err = service.Create(ctx, p, "direct-with-cart-"+stamp, write)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalAmount != 23600 {
		t.Fatalf("direct order consumed cart quantity: %d", result.TotalAmount)
	}
	after, err := carts.Get(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 1 || after.Items[0].ID != before.Items[0].ID || after.Items[0].Quantity != 5 {
		t.Fatalf("cart modified: %+v", after)
	}
	// Client versions and tenant ownership remain enforced.
	stale := write
	stale.DirectItem = &DirectItem{SKUID: fmt.Sprint(sku), SKUVersion: 2, Quantity: 1}
	if _, err = service.Create(ctx, p, "stale-"+stamp, stale); err == nil {
		t.Fatal("stale version accepted")
	}
	other := p
	other.OrgID = 2
	if _, err = service.Create(ctx, other, "other-org-"+stamp, write); err == nil {
		t.Fatal("cross-org service accepted")
	}
	// Existing cart checkout still charges and removes selected cart rows.
	normal := write
	normal.DirectItem = nil
	normal.CartItemIDs = []string{before.Items[0].ID}
	result, err = service.Create(ctx, p, "normal-cart-"+stamp, normal)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalAmount != 59000 {
		t.Fatalf("cart total=%d", result.TotalAmount)
	}
	after, err = carts.Get(ctx, p)
	if err != nil || len(after.Items) != 0 {
		t.Fatalf("cart checkout failed: %+v %v", after, err)
	}
}

func TestDirectOrderRejectsInvalidParameters(t *testing.T) {
	base := Write{ContactName: "测试客户", ContactMobile: "13800138000", ServiceAddress: "汉川市测试路6号楼203室", AppointmentDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), AppointmentSlot: "08:00"}
	for _, item := range []DirectItem{{SKUID: "1", SKUVersion: 1, Quantity: 0}, {SKUID: "1", SKUVersion: 1, Quantity: 100}, {SKUID: "bad", SKUVersion: 1, Quantity: 1}, {SKUID: "1", SKUVersion: 0, Quantity: 1}} {
		w := base
		w.DirectItem = &item
		if _, err := New(nil).Create(context.Background(), auth.Principal{}, "invalid", w); err == nil {
			t.Fatalf("accepted %+v", item)
		}
	}
	base.DirectItem = &DirectItem{SKUID: "1", SKUVersion: 1, Quantity: 1}
	base.CartItemIDs = []string{"1"}
	if _, err := New(nil).Create(context.Background(), auth.Principal{}, "mixed", base); err == nil {
		t.Fatal("mixed checkout accepted")
	}
}
