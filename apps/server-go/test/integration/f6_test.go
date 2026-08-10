package integration

import (
	"context"
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
	"log/slog"
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
	if err := db.QueryRowContext(ctx, `INSERT INTO customer(org_id,display_name,source_channel) VALUES(1,$1,'F6') RETURNING id`, fmt.Sprintf("F6 customer %d", suffix)).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, `DELETE FROM customer WHERE id=$1`, customerID)
	if err := db.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,$2,'F6','13800138000','F6 test address','REPAIR','PENDING_CONFIRMATION',100,0,1) RETURNING id`, fmt.Sprintf("F6-CROSS-%d", suffix), customerID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, `DELETE FROM customer_order WHERE id=$1`, orderID)
	c := config.Config{Env: "local", MediaDriver: "local", MediaLocalRoot: t.TempDir(), AdminUsername: "admin", AdminPassword: "password", CORSAllowedOrigins: []string{"*"}}
	handler, err := app.New(c, db, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/mini/orders/%d", orderID), nil)
	req.Header.Set("Authorization", "Bearer local-customer-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-customer order status=%d, want 404", rec.Code)
	}
}
