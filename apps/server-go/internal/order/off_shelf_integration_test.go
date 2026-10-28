package order

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestOffShelfCheckout(t *testing.T) {
	dsn := os.Getenv("ORDER_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable ORDER_TEST_DSN required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var database string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&database); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(database, "_test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	var customer, sku, availableSKU int64
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
		db.Exec(`DELETE FROM service_sku_version WHERE sku_id IN ($1,$2)`, sku, availableSKU)
		db.Exec(`DELETE FROM service_sku WHERE id IN ($1,$2)`, sku, availableSKU)
	})
	if err = db.QueryRow(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,base_price,unit,current_published_version,status) VALUES(1,1001,$1,'测试维修','测试范围','无','测试质保','FIXED',11800,'次',1,'PUBLISHED') RETURNING id`, stamp).Scan(&sku); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(catalog.PublishedSKU{ID: fmt.Sprint(sku), SKUCode: stamp, Name: "测试维修", ServiceScope: "测试范围", Exclusions: "无", WarrantyDescription: "测试质保", PriceMode: "FIXED", Price: 11800, Unit: "次", CoverImageURL: "/api/v1/media/1", PublishedVersion: 1})
	if _, err = db.Exec(`INSERT INTO service_sku_version(org_id,sku_id,version_no,snapshot_json,published_by) VALUES(1,$1,1,$2,'TEST')`, sku, snapshot); err != nil {
		t.Fatal(err)
	}
	p := auth.Principal{OrgID: 1, SubjectID: customer, Role: "CUSTOMER", Name: "测试客户"}
	service := New(db)
	write := Write{ContactName: "测试客户", ContactMobile: "13800138000", ServiceAddress: "汉川市测试路6号楼203室", AppointmentDate: time.Now().AddDate(0, 0, 1).Format("2006-01-02"), AppointmentSlot: "08:00", DirectItem: &DirectItem{SKUID: fmt.Sprint(sku), SKUVersion: 1, Quantity: 2}}

	carts := cart.New(db)
	before, err := carts.Add(ctx, p, sku, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Items) != 1 || !before.Items[0].Available || before.TotalAmount != 23600 {
		t.Fatalf("published cart: %+v", before)
	}
	if err = catalog.New(db).OffShelf(ctx, sku); err != nil {
		t.Fatal(err)
	}
	unavailable, err := carts.Get(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(unavailable.Items) != 1 || unavailable.Items[0].Available || unavailable.TotalAmount != 0 || unavailable.ItemCount != 2 {
		t.Fatalf("off-shelf cart: %+v", unavailable)
	}
	assertUnavailable := func(err error) {
		t.Helper()
		var apiError *httpx.Error
		if !errors.As(err, &apiError) || apiError.Code != "SKU_NOT_AVAILABLE" {
			t.Fatalf("expected SKU_NOT_AVAILABLE, got %v", err)
		}
	}
	_, err = carts.Add(ctx, p, sku, 1)
	assertUnavailable(err)
	_, err = service.Create(ctx, p, "off-shelf-direct-"+stamp, write)
	assertUnavailable(err)
	normal := write
	normal.DirectItem = nil
	normal.CartItemIDs = []string{before.Items[0].ID}
	_, err = service.Create(ctx, p, "off-shelf-selected-"+stamp, normal)
	assertUnavailable(err)
	normal.CartItemIDs = nil
	_, err = service.Create(ctx, p, "off-shelf-all-"+stamp, normal)
	assertUnavailable(err)
	var orders int
	if err = db.QueryRow(`SELECT COUNT(*) FROM customer_order WHERE customer_id=$1`, customer).Scan(&orders); err != nil || orders != 0 {
		t.Fatalf("rejected checkout created orders: %d, %v", orders, err)
	}
	after, err := carts.Get(ctx, p)
	if err != nil || len(after.Items) != 1 || after.Items[0].ID != before.Items[0].ID {
		t.Fatalf("rejected checkout modified cart: %+v %v", after, err)
	}
	// A second published service can be checked out while the off-shelf item stays in the cart.
	if err = db.QueryRow(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,base_price,unit,current_published_version,status) VALUES(1,1001,$1,'可购买服务','测试范围','无','测试质保','FIXED',11800,'次',1,'PUBLISHED') RETURNING id`, "available-"+stamp).Scan(&availableSKU); err != nil {
		t.Fatal(err)
	}

	var availableSnapshot catalog.PublishedSKU
	if err = json.Unmarshal(snapshot, &availableSnapshot); err != nil {
		t.Fatal(err)
	}
	availableSnapshot.ID = fmt.Sprint(availableSKU)
	availableSnapshot.SKUCode = "available-" + stamp
	availableSnapshot.Name = "可购买服务"
	raw, err := json.Marshal(availableSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO service_sku_version(org_id,sku_id,version_no,snapshot_json,published_by) VALUES(1,$1,1,$2,'TEST')`, availableSKU, raw); err != nil {
		t.Fatal(err)
	}
	mixed, err := carts.Add(ctx, p, availableSKU, 1)
	if err != nil || mixed.TotalAmount != 11800 {
		t.Fatalf("mixed cart: %+v %v", mixed, err)
	}
	for _, item := range mixed.Items {
		if item.Available {
			normal.CartItemIDs = []string{item.ID}
		}
	}
	result, err := service.Create(ctx, p, "available-selected-"+stamp, normal)
	if err != nil || result.TotalAmount != 11800 {
		t.Fatalf("available checkout: %+v %v", result, err)
	}
	after, err = carts.Get(ctx, p)
	if err != nil || len(after.Items) != 1 || after.Items[0].Available {
		t.Fatalf("off-shelf item should remain removable: %+v %v", after, err)
	}
	// Restoring publication makes the retained item purchasable again.
	if _, err = db.Exec(`UPDATE service_sku SET status='PUBLISHED' WHERE id=$1`, sku); err != nil {
		t.Fatal(err)
	}
	after, err = carts.Get(ctx, p)
	if err != nil || !after.Items[0].Available || after.TotalAmount != 23600 {
		t.Fatalf("restored cart: %+v %v", after, err)
	}
	if err = catalog.New(db).OffShelf(ctx, sku); err != nil {
		t.Fatal(err)
	}
	var itemID int64
	if _, err = fmt.Sscan(before.Items[0].ID, &itemID); err != nil {
		t.Fatal(err)
	}
	after, err = carts.Delete(ctx, p, itemID)
	if err != nil || len(after.Items) != 0 {
		t.Fatalf("off-shelf item deletion: %+v %v", after, err)
	}
}
