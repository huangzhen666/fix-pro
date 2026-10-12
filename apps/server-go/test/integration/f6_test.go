package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/fixpro/server/internal/app"
	"github.com/fixpro/server/internal/platform/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
)

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("FIXPRO_INTEGRATION") != "1" {
		t.Skip("set FIXPRO_INTEGRATION=1 to run against PostgreSQL")
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://fixpro:fixpro-local@localhost:5433/fix_pro?sslmode=disable&timezone=UTC"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestF6MigrationAndConcurrentVersion(t *testing.T) {
	db := integrationDB(t)
	var version int
	if err := db.QueryRow(`SELECT version FROM schema_migrations LIMIT 1`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 2 {
		t.Fatalf("migration version=%d, want >=2", version)
	}
	var orderID, workID int64
	suffix := time.Now().UnixNano()
	if err := db.QueryRow(`INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,1,'F6','13800138000','F6 test address','REPAIR','FULFILLING',100,0,1) RETURNING id`, fmt.Sprintf("F6-%d", suffix)).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM customer_order WHERE id=$1`, orderID)
	if err := db.QueryRow(`INSERT INTO work_order(org_id,work_order_no,order_id,status,priority) VALUES(1,$1,$2,'PENDING_DISPATCH','NORMAL') RETURNING id`, fmt.Sprintf("F6-WO-%d", suffix), orderID).Scan(&workID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,fault_description,unit_price,quantity,subtotal_amount) VALUES(1,$1,1001,1,'F6','F6','次','scope','none','none','description',100,1,100)`, orderID); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan int64, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := db.Exec(`UPDATE work_order SET version=version+1 WHERE id=$1 AND version=0`, workID)
			if err != nil {
				t.Error(err)
				return
			}
			n, _ := res.RowsAffected()
			results <- n
		}()
	}
	wg.Wait()
	close(results)
	var success int
	for n := range results {
		if n == 1 {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent optimistic updates succeeded %d times, want 1", success)
	}
}

func TestF6CustomerCannotReadOtherCustomerOrder(t *testing.T) {
	db := integrationDB(t)
	ctx := context.Background()
	var customerID, orderID int64
	suffix := time.Now().UnixNano()
	if err := db.QueryRowContext(ctx, `INSERT INTO customer(org_id,display_name,source_channel,mobile_cipher,mobile_hash) VALUES(1,$1::text,'F6',decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, fmt.Sprintf("F6 customer %d", suffix)).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, `DELETE FROM customer WHERE id=$1`, customerID)
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,$2,'F6','13800138000','F6 test address','REPAIR','PENDING_CONFIRMATION',100,0,1) RETURNING id`, fmt.Sprintf("F6-CROSS-%d", suffix), customerID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	var ownID int64
	if err := db.QueryRow(`INSERT INTO customer(org_id,display_name,source_channel,mobile_cipher,mobile_hash) VALUES(1,$1::text,'F6',decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, fmt.Sprintf("F6 owner %d", suffix)).Scan(&ownID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM customer WHERE id=$1`, ownID)
	token := fmt.Sprintf("F6-session-%d", suffix)
	digest := sha256.Sum256([]byte(token))
	if _, err := db.Exec(`INSERT INTO customer_session(org_id,customer_id,appid,openid,token_hash,expires_at) VALUES(1,$1,'test-app','test-env',$2,now()+interval '1 hour')`, ownID, digest[:]); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM customer_session WHERE customer_id=$1`, ownID)
	c := config.Config{WechatAppID: "test-app", Env: "local", MediaDriver: "local", MediaLocalRoot: t.TempDir(), AdminUsername: "admin", AdminPassword: "password", CORSAllowedOrigins: []string{"*"}}
	handler, err := app.New(c, db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/mini/orders/%d", orderID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-customer order status=%d, want 404", rec.Code)
	}

	// Customer Bearer cannot authenticate administrator APIs.
	req = httptest.NewRequest("GET", "/api/v1/admin/customers/overview", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("customer token used as admin: %d", rec.Code)
	}
	for i, tc := range []struct {
		codes            []string
		overview, orders int
	}{
		{[]string{"order.view"}, 403, 403},
		{[]string{"customer.view"}, 200, 403},
		{[]string{"customer.view", "order.view"}, 200, 404},
	} {
		username := fmt.Sprintf("F6-admin-%d-%d", suffix, i)
		var user, role int64
		if err := db.QueryRow(`INSERT INTO admin_user(org_id,username,display_name,password_hash,must_change_password) VALUES(1,$1,$1,'fixture',false) RETURNING id`, username).Scan(&user); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(`DELETE FROM admin_user WHERE id=$1`, user)
		if err := db.QueryRow(`INSERT INTO admin_role(org_id,role_code,name) VALUES(1,$1,$1) RETURNING id`, username).Scan(&role); err != nil {
			t.Fatal(err)
		}
		defer db.Exec(`DELETE FROM admin_role WHERE id=$1`, role)
		defer db.Exec(`DELETE FROM admin_role_permission WHERE role_id=$1`, role)
		db.Exec(`INSERT INTO admin_user_role(org_id,user_id,role_id) VALUES(1,$1,$2)`, user, role)
		for _, code := range tc.codes {
			if _, err := db.Exec(`INSERT INTO admin_role_permission(org_id,role_id,permission_id) SELECT 1,$1,id FROM admin_permission WHERE permission_code=$2`, role, code); err != nil {
				t.Fatal(err)
			}
		}
		adminToken := username + "-token"
		adminDigest := sha256.Sum256([]byte(adminToken))
		csrf := sha256.Sum256([]byte("fixture-csrf"))
		if _, err := db.Exec(`INSERT INTO admin_user_session(org_id,user_id,token_hash,csrf_token_hash,expires_at) VALUES(1,$1,$2,$3,now()+interval '1 hour')`, user, adminDigest[:], csrf[:]); err != nil {
			t.Fatal(err)
		}
		// Direct SQL fixtures bypass the admin mutation hooks that normally invalidate Casbin.
		scenarioHandler, err := app.New(c, db, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]int{"/api/v1/admin/customers/overview": tc.overview, fmt.Sprintf("/api/v1/admin/customers/%d/orders", customerID): tc.orders} {
			req = httptest.NewRequest("GET", path, nil)
			req.AddCookie(&http.Cookie{Name: "fixpro_admin_session", Value: adminToken})
			rec = httptest.NewRecorder()
			scenarioHandler.ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("permission combination %d path %s: %d want %d", i, path, rec.Code, want)
			}
		}
	}
}
