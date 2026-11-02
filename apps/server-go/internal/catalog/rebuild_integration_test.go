package catalog

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRebuildGuardAtomicityAndScope(t *testing.T) {
	dsn := os.Getenv("CATALOG_TEST_DSN")
	if dsn == "" {
		t.Skip("CATALOG_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var name string
	if e = db.QueryRow(`SELECT current_database()`).Scan(&name); e != nil || !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable _test database required", e)
	}
	ctx := context.Background()
	c, e := db.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	schema := fmt.Sprintf("sku_migration_%d", time.Now().UnixNano())
	exec := func(q string) {
		t.Helper()
		if _, e = c.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	exec(`CREATE SCHEMA ` + schema)
	defer c.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	exec(`SET search_path TO ` + schema)
	files, e := filepath.Glob("../../db/migrations/*.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	var migration string
	for _, file := range files {
		b, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(file, "000020_") {
			migration = string(b)
			break
		}
		exec(string(b))
	}
	if migration == "" {
		t.Fatal("missing rebuild migration")
	}
	exec(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,status) VALUES(1,1001,'OLD','Old SKU','old scope','old exclusion','old warranty','FIXED','DRAFT')`)
	exec(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'ADMIN',1,'SKU_IMAGE','IMAGE','old.png','retained-file','image/png',1,'READY')`)
	exec(`INSERT INTO service_sku_media(org_id,sku_id,media_id,media_role) SELECT 1,s.id,m.id,'COVER' FROM service_sku s CROSS JOIN media_asset m WHERE s.sku_code='OLD' AND m.object_key='retained-file'`)
	exec(`INSERT INTO service_sku_version(org_id,sku_id,version_no,snapshot_json,published_by) SELECT 1,id,1,'{}','test' FROM service_sku WHERE sku_code='OLD'`)
	exec(`INSERT INTO shopping_cart(org_id,customer_id) VALUES(1,999)`)
	exec(`INSERT INTO shopping_cart_item(org_id,cart_id,sku_id,sku_version,quantity,unit_price) SELECT 1,c.id,s.id,1,1,100 FROM shopping_cart c CROSS JOIN service_sku s WHERE s.sku_code='OLD'`)
	exec(`INSERT INTO shopping_cart_item_media(org_id,cart_item_id,media_id) SELECT 1,c.id,m.id FROM shopping_cart_item c CROSS JOIN media_asset m WHERE m.object_key='retained-file'`)
	exec(`INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,'GUARD',999,'测试','13800138000','测试地址','REPAIR','PENDING_CONFIRMATION',100,0,1)`)
	if _, e = c.ExecContext(ctx, migration); e == nil {
		t.Fatal("rebuild accepted non-empty orders")
	}
	exec(`ROLLBACK`)
	var n int
	if e = c.QueryRowContext(ctx, `SELECT count(*) FROM service_sku`).Scan(&n); e != nil || n != 1 {
		t.Fatal("guard deleted data", n, e)
	}
	exec(`DELETE FROM customer_order WHERE order_no='GUARD'`)
	injected := strings.Replace(migration, "COMMIT;", "DO $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$; COMMIT;", 1)
	if _, e = c.ExecContext(ctx, injected); e == nil {
		t.Fatal("failure injection did not fail")
	}
	exec(`ROLLBACK`)
	if e = c.QueryRowContext(ctx, `SELECT count(service_scope) FROM service_sku`).Scan(&n); e != nil || n != 1 {
		t.Fatal("DDL/data rollback failed", n, e)
	}
	exec(migration)
	for _, table := range []string{"service_sku", "service_sku_media", "service_sku_version", "shopping_cart_item", "shopping_cart_item_media"} {
		if e = c.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); e != nil || n != 0 {
			t.Fatal("old refs retained", table, n, e)
		}
	}
	if e = c.QueryRowContext(ctx, `SELECT count(*) FROM media_asset WHERE object_key='retained-file'`).Scan(&n); e != nil || n != 1 {
		t.Fatal("media deleted", n, e)
	}
	if e = c.QueryRowContext(ctx, `SELECT count(*) FROM service_category`).Scan(&n); e != nil || n == 0 {
		t.Fatal("categories deleted", n, e)
	}
	if e = c.QueryRowContext(ctx, `SELECT version FROM shopping_cart WHERE customer_id=999`).Scan(&n); e != nil || n != 1 {
		t.Fatal("cart version not advanced", n, e)
	}
}
