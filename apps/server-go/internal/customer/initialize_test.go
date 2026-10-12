package customer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCleanupMediaRetry(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_CLEANUP_TEST_DSN")
	if dsn == "" {
		t.Skip("fresh disposable CUSTOMER_CLEANUP_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	var name string
	db.QueryRow(`SELECT current_database()`).Scan(&name)
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable test DB required")
	}
	var initialized bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM customer_initialization)`).Scan(&initialized)
	if initialized {
		t.Skip("cleanup test requires fresh migrated disposable DB")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "customer_avatar/blocked")
	os.MkdirAll(blocked, 0750)
	os.WriteFile(filepath.Join(blocked, "child"), []byte("block removal"), 0600)
	retained := filepath.Join(root, "customer_avatar/retained.png")
	os.WriteFile(retained, []byte("shared catalog fixture"), 0600)
	var mid int64
	e = db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'CUSTOMER',1,'SKU_IMAGE','IMAGE','retained.png','customer_avatar/retained.png','image/png',7,'READY') RETURNING id`).Scan(&mid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO service_sku(org_id,category_id,sku_code,name,service_scope,exclusions,warranty_description,price_mode,cover_media_id,status) VALUES(1,1001,'CLEANUP-RETAIN','retained','scope','none','none','FIXED',$1,'DRAFT')`, mid)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'CUSTOMER',1,'CUSTOMER_AVATAR','IMAGE','blocked.png','customer_avatar/blocked','image/png',7,'READY');`)
	if e != nil {
		t.Fatal(e)
	}
	// Circular work-order confirmation FK is cleared before dependent deletes.
	_, e = db.Exec(`INSERT INTO customer_order(id,org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(99001,1,'CLEANUP-ORDER',1,'测试','13800138000','测试地址','REPAIR','FULFILLING',1,0,1); INSERT INTO work_order(id,org_id,work_order_no,order_id,status,priority) VALUES(99001,1,'CLEANUP-WORK',99001,'FINISHED','NORMAL');`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = db.Exec(`INSERT INTO customer_service_confirmation(id,org_id,work_order_id,operator_id,decision,note) VALUES(99001,1,99001,1,'NO_SECOND_VISIT','测试'); UPDATE work_order SET customer_service_confirmation_id=99001 WHERE id=99001`)
	if e != nil {
		t.Fatal(e)
	}
	preview, e := Initialize(ctx, db, root, false, false)
	if e != nil || preview.Completed || len(preview.Objects) != 1 {
		t.Fatal("preview/shared exclusion", preview, e)
	}
	done, e := Initialize(ctx, db, root, true, false)
	if e == nil || !done.Completed {
		t.Fatal("media failure must retain committed manifest", done, e)
	}
	var customers, orders, work int
	db.QueryRow(`SELECT count(*) FROM customer`).Scan(&customers)
	db.QueryRow(`SELECT count(*) FROM customer_order`).Scan(&orders)
	db.QueryRow(`SELECT count(*) FROM work_order`).Scan(&work)
	if customers+orders+work != 0 {
		t.Fatal("dependent data not cleared")
	}
	if _, e = Initialize(ctx, db, root, true, false); e == nil {
		t.Fatal("second destructive cleanup accepted")
	}
	os.Remove(filepath.Join(blocked, "child"))
	if _, e = Initialize(ctx, db, root, false, true); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(retained); e != nil {
		t.Fatal("shared catalog media removed")
	}
	if _, e = os.Stat(blocked); !os.IsNotExist(e) {
		t.Fatal("private file retry failed")
	}
	var preserved int
	db.QueryRow(`SELECT count(*) FROM service_sku WHERE sku_code='CLEANUP-RETAIN'`).Scan(&preserved)
	if preserved != 1 {
		t.Fatal("catalog removed")
	}
}
