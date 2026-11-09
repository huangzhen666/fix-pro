package payment

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func paymentDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("PAYMENT_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable PAYMENT_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	var name string
	if e = db.QueryRow(`SELECT current_database()`).Scan(&name); e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable test database required")
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func fixture(t *testing.T, db *sql.DB) (auth.Principal, int64) {
	t.Helper()
	var customer, id int64
	stamp := randomID("TEST")
	e := db.QueryRow(`INSERT INTO customer(org_id,display_name,mobile_cipher,mobile_hash) VALUES(1,$1::text,decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, stamp).Scan(&customer)
	if e != nil {
		t.Fatal(e)
	}
	e = db.QueryRow(`INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,item_count,appointment_at,appointment_slot,payment_expires_at) VALUES(1,$1,$2,'测试客户','13800138000','测试地址','SERVICE','PENDING_CONFIRMATION',100,1,now()+interval '1 day','08:00',now()+interval '15 minutes') RETURNING id`, stamp, customer).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM payment_event WHERE event_key LIKE '%' || (SELECT out_trade_no FROM payment_order WHERE order_id=$1 LIMIT 1) || '%'`, id)
		db.Exec(`DELETE FROM audit_log WHERE resource_type='PAYMENT' AND resource_id IN(SELECT id::text FROM payment_order WHERE order_id=$1)`, id)
		db.Exec(`DELETE FROM audit_log WHERE resource_type='ORDER' AND resource_id=$1`, fmt.Sprint(id))
		db.Exec(`DELETE FROM payment_refund WHERE payment_id IN(SELECT id FROM payment_order WHERE order_id=$1)`, id)
		db.Exec(`DELETE FROM payment_order WHERE order_id=$1`, id)
		db.Exec(`DELETE FROM order_status_history WHERE order_id=$1`, id)
		db.Exec(`DELETE FROM customer_order WHERE id=$1`, id)
		db.Exec(`DELETE FROM customer WHERE id=$1`, customer)
	})
	return auth.Principal{OrgID: 1, SubjectID: customer, Role: "CUSTOMER"}, id
}
func TestMockPaymentRefundIsolationAndConcurrency(t *testing.T) {
	db := paymentDB(t)
	p, id := fixture(t, db)
	ctx := context.Background()
	s := New(db, config.Payment{MockEnabled: true, OrderTTL: 15 * time.Minute}, "local")
	prepared, e := s.Prepare(ctx, p, id, "MOCK")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Prepare(ctx, p, id, "MOCK")
	if e != nil || again.Payment.ID != prepared.Payment.ID {
		t.Fatal("prepay not idempotent", e)
	}
	other := p
	other.SubjectID++
	if _, e = s.Overview(ctx, other, id, false); e == nil {
		t.Fatal("foreign customer read payment")
	}
	if _, e = s.Prepare(ctx, other, id, "MOCK"); e == nil {
		t.Fatal("foreign customer prepared payment")
	}
	if e = s.Mock(ctx, other, id, "SUCCESS"); e == nil {
		t.Fatal("foreign customer paid")
	}
	if _, e = s.Prepare(ctx, p, id, "WECHAT"); e == nil {
		t.Fatal("disabled real channel allowed")
	}
	if e = s.Mock(ctx, p, id, "CANCEL"); e != nil {
		t.Fatal(e)
	}
	o, e := s.Overview(ctx, p, id, false)
	if e != nil || o.Paid != 0 {
		t.Fatal("cancel changed money", e)
	}
	admin := auth.Principal{OrgID: 1, Role: "ADMIN", AdminUserID: 7, Name: "测试管理员"}
	_, e = fulfillment.New(db, nil).ConfirmOrder(ctx, admin, randomID("K"), id, fulfillment.ConfirmRequest{Version: 0, Priority: "NORMAL"})
	if e == nil || !strings.Contains(e.Error(), "ORDER_PAYMENT_REQUIRED") {
		t.Fatalf("unpaid confirmation: %v", e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Mock(ctx, p, id, "SUCCESS") }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	o, e = s.Overview(ctx, p, id, false)
	if e != nil || o.Paid != 100 || o.Status != "PAID" {
		t.Fatalf("duplicate payment: %+v %v", o, e)
	}
	// Two concurrent 70-cent refunds must not reserve more than the 100-cent receipt.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Refund(ctx, admin, id, 70, "测试退款", randomID("K"))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent refunds accepted=%d", success)
	}
	key := randomID("K")
	r, e := s.Refund(ctx, admin, id, 30, "退还剩余款项", key)
	if e != nil {
		t.Fatal(e)
	}
	r2, e := s.Refund(ctx, admin, id, 30, "退还剩余款项", key)
	if e != nil || r.ID != r2.ID {
		t.Fatal("refund replay", e)
	}
	if _, e = s.Refund(ctx, admin, id, 20, "变更请求", key); e == nil {
		t.Fatal("idempotency conflict allowed")
	}
	o, e = s.Overview(ctx, p, id, false)
	if e != nil || o.Refunded != 100 || o.Paid != 100 || o.Status != "REFUNDED" {
		t.Fatalf("refund facts %+v %v", o, e)
	}
	if _, e = s.Refund(ctx, admin, id, 1, "超额", randomID("K")); e == nil {
		t.Fatal("over-refund allowed")
	}
	if e = s.Reconcile(ctx); e != nil {
		t.Fatal("reconcile", e)
	}
}
func TestExpiryAndNoMockInProduction(t *testing.T) {
	db := paymentDB(t)
	p, id := fixture(t, db)
	ctx := context.Background()
	s := New(db, config.Payment{MockEnabled: true, OrderTTL: 15 * time.Minute}, "local")
	prepared, e := s.Prepare(ctx, p, id, "MOCK")
	if e != nil {
		t.Fatal(e)
	}
	prod := New(db, config.Payment{MockEnabled: true}, "production")
	if _, e = prod.Prepare(ctx, p, id, "MOCK"); e == nil {
		t.Fatal("prod mock prepare allowed")
	}
	if e = prod.Mock(ctx, p, id, "SUCCESS"); e == nil {
		t.Fatal("prod mock apply allowed")
	}
	if _, e = db.Exec(`UPDATE payment_order SET expires_at=now()-interval '1 minute' WHERE id=$1::bigint`, prepared.Payment.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Mock(ctx, p, id, "SUCCESS"); e == nil {
		t.Fatal("expired mock accepted")
	}
	if e = s.Reconcile(ctx); e != nil {
		t.Fatal(e)
	}
	var status string
	if e = db.QueryRow(`SELECT status FROM customer_order WHERE id=$1`, id).Scan(&status); e != nil || status != "CANCELLED" {
		t.Fatalf("expiry %s %v", status, e)
	}
}
func TestVerifiedRealEventsAndLateReceipt(t *testing.T) {
	db := paymentDB(t)
	p, id := fixture(t, db)
	ctx := context.Background()
	c := config.Payment{WechatEnabled: true, AppID: "app", MerchantID: "merchant", TicketSecret: strings.Repeat("t", 32), OrderTTL: 15 * time.Minute}
	s := New(db, c, "staging")
	if _, e := db.Exec(`INSERT INTO customer_wechat_identity(org_id,customer_id,appid,openid) VALUES(1,$1,'app','trusted-openid')`, p.SubjectID); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM customer_wechat_identity WHERE customer_id=$1`, p.SubjectID) })
	prepared, e := s.Prepare(ctx, p, id, "WECHAT")
	if e != nil {
		t.Fatal(e)
	}
	data, e := s.Authorize(ctx, prepared.Ticket)
	if e != nil || data["payer"].(map[string]string)["openid"] != "trusted-openid" {
		t.Fatalf("authorize %v %v", data, e)
	}
	if _, e = s.Authorize(ctx, prepared.Ticket); e == nil {
		t.Fatal("concurrent prepay lease not enforced")
	}
	if e = s.SavePrepay(ctx, prepared.Ticket, "wx-cached-prepay"); e != nil {
		t.Fatal(e)
	}
	cached, e := s.Authorize(ctx, prepared.Ticket)
	if e != nil || cached["prepay_id"] != "wx-cached-prepay" {
		t.Fatal("prepay not reused", e)
	}
	ev := Event{Kind: "PAYMENT", Channel: "WECHAT", OutTradeNo: prepared.Payment.OutTradeNo, TransactionID: randomID("WX"), Amount: 100, Total: 100, Currency: "CNY", AppID: "app", MerchantID: "merchant", Status: "SUCCESS"}
	bad := ev
	bad.Amount = 1
	if e = s.Apply(ctx, bad); e == nil {
		t.Fatal("wrong amount accepted")
	}
	bad = ev
	bad.MerchantID = "other"
	if e = s.Apply(ctx, bad); e == nil {
		t.Fatal("foreign merchant accepted")
	}
	if _, e = db.Exec(`UPDATE customer_order SET status='CANCELLED' WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(ctx, ev); e != nil {
		t.Fatal(e)
	}
	if e = s.Apply(ctx, ev); e != nil {
		t.Fatal("replayed real receipt", e)
	}
	var status, alert string
	var paid int64
	if e = db.QueryRow(`SELECT o.status,o.paid_amount,p.last_error FROM customer_order o JOIN payment_order p ON p.order_id=o.id WHERE o.id=$1`, id).Scan(&status, &paid, &alert); e != nil || status != "CANCELLED" || paid != 100 || alert != "LATE_PAYMENT_REQUIRES_REFUND" {
		t.Fatalf("late payment %s %d %s %v", status, paid, alert, e)
	}
}

func TestFulfillmentPaymentGateAndLegacyExemption(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	s := New(db, config.Payment{MockEnabled: true, OrderTTL: 15 * time.Minute}, "local")
	for _, legacy := range []bool{false, true} {
		p, id := fixture(t, db)
		_, e := db.Exec(`INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,demand_description,unit_price,quantity,subtotal_amount) VALUES(1,$1,0,1,'TEST','测试服务','次','',100,1,100)`, id)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() {
			db.Exec(`DELETE FROM work_order_status_history WHERE work_order_id IN(SELECT id FROM work_order WHERE order_id=$1)`, id)
			db.Exec(`DELETE FROM work_order_item WHERE work_order_id IN(SELECT id FROM work_order WHERE order_id=$1)`, id)
			db.Exec(`DELETE FROM work_order WHERE order_id=$1`, id)
			db.Exec(`DELETE FROM order_item WHERE order_id=$1`, id)
		})
		admin := auth.Principal{OrgID: 1, Role: "ADMIN", Name: "测试管理员"}
		if legacy {
			if _, e = db.Exec(`UPDATE customer_order SET payment_required=false WHERE id=$1`, id); e != nil {
				t.Fatal(e)
			}
		} else {
			if _, e = s.Prepare(ctx, p, id, "MOCK"); e != nil {
				t.Fatal(e)
			}
			if e = s.Mock(ctx, p, id, "SUCCESS"); e != nil {
				t.Fatal(e)
			}
		}
		var version int
		if e = db.QueryRow(`SELECT version FROM customer_order WHERE id=$1`, id).Scan(&version); e != nil {
			t.Fatal(e)
		}
		key := randomID("K")
		out, e := fulfillment.New(db, nil).ConfirmOrder(ctx, admin, key, id, fulfillment.ConfirmRequest{Version: version, Priority: "NORMAL"})
		if e != nil || len(out.WorkOrders) != 1 {
			t.Fatalf("legacy=%v confirmation %+v %v", legacy, out, e)
		}
		again, e := fulfillment.New(db, nil).ConfirmOrder(ctx, admin, key, id, fulfillment.ConfirmRequest{Version: version, Priority: "NORMAL"})
		if e != nil || again.WorkOrders[0].ID != out.WorkOrders[0].ID {
			t.Fatal("confirmation duplicate", e)
		}
	}
}

func TestMigrationPreservesHistoryAndRefusesDeletingMoneyFacts(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	schema := strings.ToLower(randomID("payment_migration_"))
	if _, e = tx.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"; SET LOCAL search_path TO "%s"`, schema, schema)); e != nil {
		t.Fatal(e)
	}
	baseline := `CREATE TABLE customer_order(id BIGINT PRIMARY KEY,paid_amount BIGINT NOT NULL DEFAULT 0);
 INSERT INTO customer_order VALUES(1,35);
 CREATE TABLE admin_permission(id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,permission_code TEXT,resource TEXT,action TEXT,permission_type TEXT,route_path TEXT,sort_order INTEGER);
 CREATE TABLE admin_role(org_id BIGINT,id BIGINT,role_code TEXT);
 CREATE TABLE admin_role_permission(org_id BIGINT,role_id BIGINT,permission_id BIGINT,UNIQUE(org_id,role_id,permission_id));`
	if _, e = tx.ExecContext(ctx, baseline); e != nil {
		t.Fatal(e)
	}
	up, e := os.ReadFile("../../db/migrations/000022_payment.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(ctx, string(up)); e != nil {
		t.Fatal(e)
	}
	var required bool
	var paid int64
	if e = tx.QueryRowContext(ctx, `SELECT payment_required,paid_amount FROM customer_order WHERE id=1`).Scan(&required, &paid); e != nil || required || paid != 35 {
		t.Fatalf("legacy changed: %v %d %v", required, paid, e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO customer_order(id) VALUES(2)`); e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRowContext(ctx, `SELECT payment_required FROM customer_order WHERE id=2`).Scan(&required); e != nil || !required {
		t.Fatal("new order not payable", e)
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO payment_order(org_id,order_id,customer_id,channel,out_trade_no,amount,expires_at) VALUES(1,2,1,'MOCK','rollback-protection',100,now())`); e != nil {
		t.Fatal(e)
	}
	down, e := os.ReadFile("../../db/migrations/000022_payment.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.ExecContext(ctx, string(down)); e == nil || !strings.Contains(e.Error(), "payment facts exist") {
		t.Fatal("rollback deleted payment facts", e)
	}
}

func TestPaymentRacingCancellationAndCloseNeverLosesReceipt(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	s := New(db, config.Payment{WechatEnabled: true, AppID: "app", MerchantID: "merchant", TicketSecret: strings.Repeat("t", 32)}, "staging")
	for _, mode := range []string{"cancel", "close"} {
		p, id := fixture(t, db)
		pay, e := s.Prepare(ctx, p, id, "WECHAT")
		if e != nil {
			t.Fatal(e)
		}
		success := Event{Kind: "PAYMENT", Channel: "WECHAT", OutTradeNo: pay.Payment.OutTradeNo, TransactionID: randomID("WX"), Amount: 100, Total: 100, Currency: "CNY", AppID: "app", MerchantID: "merchant", Status: "SUCCESS"}
		var wg sync.WaitGroup
		receipt := make(chan error, 1)
		wg.Add(2)
		go func() { defer wg.Done(); receipt <- s.Apply(ctx, success) }()
		go func() {
			defer wg.Done()
			if mode == "cancel" {
				_, _ = fulfillment.New(db, nil).RejectOrder(ctx, auth.Principal{OrgID: 1, Role: "ADMIN"}, randomID("K"), id, fulfillment.OrderRejectRequest{Version: 0, Reason: "竞态取消测试"})
			} else {
				closed := success
				closed.Status = "CLOSED"
				closed.TransactionID = "CLOSED_" + success.OutTradeNo
				_ = s.Apply(ctx, closed)
			}
		}()
		wg.Wait()
		if e = <-receipt; e != nil {
			t.Fatal("receipt lost", e)
		}
		out, e := s.Overview(ctx, p, id, false)
		if e != nil || out.Paid != 100 || out.Payments[0].Status != "SUCCESS" {
			t.Fatalf("%s: %+v %v", mode, out, e)
		}
	}
}
