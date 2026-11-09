package payment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Service struct {
	db     *sql.DB
	cfg    config.Payment
	env    string
	bridge *bridge
}

func New(db *sql.DB, c config.Payment, env string) *Service {
	return &Service{db, c, env, newBridge(c)}
}
func randomID(prefix string) string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return prefix + hex.EncodeToString(b[:])
}
func (s *Service) mockAllowed() bool { return s.env == "local" && s.cfg.MockEnabled }
func (s *Service) channelAllowed(channel string) bool {
	return channel == "MOCK" && s.mockAllowed() || channel == "WECHAT" && s.cfg.WechatEnabled
}

type Payment struct {
	LastError     string     `json:"lastError,omitempty"`
	ID            string     `json:"id"`
	OrderID       string     `json:"orderId"`
	Channel       string     `json:"channel"`
	OutTradeNo    string     `json:"outTradeNo"`
	TransactionID string     `json:"transactionId"`
	Amount        int64      `json:"amount"`
	Status        string     `json:"status"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	PaidAt        *time.Time `json:"paidAt"`
}
type Refund struct {
	ID          string    `json:"id"`
	OutRefundNo string    `json:"outRefundNo"`
	Amount      int64     `json:"amount"`
	Reason      string    `json:"reason"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Overview struct {
	Required       bool       `json:"required"`
	Status         string     `json:"status"`
	Total          int64      `json:"totalAmount"`
	Paid           int64      `json:"paidAmount"`
	Refunded       int64      `json:"refundedAmount"`
	RefundReserved int64      `json:"refundReserved"`
	ExpiresAt      *time.Time `json:"expiresAt"`
	Payments       []Payment  `json:"payments"`
	Refunds        []Refund   `json:"refunds"`
	MockEnabled    bool       `json:"mockEnabled"`
	WechatEnabled  bool       `json:"wechatEnabled"`
}
type Prepared struct {
	Payment       Payment `json:"payment"`
	Ticket        string  `json:"ticket,omitempty"`
	CloudEnv      string  `json:"cloudEnv,omitempty"`
	CloudFunction string  `json:"cloudFunction,omitempty"`
}

const paymentColumns = `id::text,order_id::text,channel,out_trade_no,COALESCE(transaction_id,''),amount,status,expires_at,paid_at,COALESCE(last_error,'')`

type scanner interface{ Scan(...any) error }

func scanPayment(r scanner) (Payment, error) {
	var p Payment
	e := r.Scan(&p.ID, &p.OrderID, &p.Channel, &p.OutTradeNo, &p.TransactionID, &p.Amount, &p.Status, &p.ExpiresAt, &p.PaidAt, &p.LastError)
	return p, e
}
func (s *Service) Overview(ctx context.Context, p auth.Principal, orderID int64, admin bool) (Overview, error) {
	out := Overview{Payments: []Payment{}, Refunds: []Refund{}, MockEnabled: s.mockAllowed(), WechatEnabled: s.cfg.WechatEnabled}
	var owner int64
	e := s.db.QueryRowContext(ctx, `SELECT customer_id,payment_required,total_amount,paid_amount,refunded_amount,payment_expires_at FROM customer_order WHERE org_id=$1 AND id=$2`, p.OrgID, orderID).Scan(&owner, &out.Required, &out.Total, &out.Paid, &out.Refunded, &out.ExpiresAt)
	if e == sql.ErrNoRows || e == nil && !admin && owner != p.SubjectID {
		return out, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	}
	if e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT `+paymentColumns+` FROM payment_order WHERE org_id=$1 AND order_id=$2 ORDER BY id DESC`, p.OrgID, orderID)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		x, e := scanPayment(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Payments = append(out.Payments, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rr, e := s.db.QueryContext(ctx, `SELECT r.id::text,r.out_refund_no,r.amount,r.reason,r.status,r.created_at FROM payment_refund r JOIN payment_order p ON p.id=r.payment_id WHERE r.org_id=$1 AND p.order_id=$2 ORDER BY r.id DESC`, p.OrgID, orderID)
	if e != nil {
		return out, e
	}
	defer rr.Close()
	for rr.Next() {
		var x Refund
		if e = rr.Scan(&x.ID, &x.OutRefundNo, &x.Amount, &x.Reason, &x.Status, &x.CreatedAt); e != nil {
			return out, e
		}
		out.Refunds = append(out.Refunds, x)
		if x.Status == "PROCESSING" || x.Status == "ABNORMAL" {
			out.RefundReserved += x.Amount
		}
	}
	out.Status = "UNPAID"
	if !out.Required {
		out.Status = "EXEMPT"
	}
	if out.Paid > 0 {
		out.Status = "PAID"
	}
	if out.Refunded > 0 {
		out.Status = "PARTIALLY_REFUNDED"
	}
	if out.Paid > 0 && out.Refunded >= out.Paid {
		out.Status = "REFUNDED"
	}
	if out.RefundReserved > 0 {
		out.Status = "REFUNDING"
	}
	return out, rr.Err()
}
func (s *Service) Prepare(ctx context.Context, p auth.Principal, orderID int64, channel string) (Prepared, error) {
	var out Prepared
	if channel == "WECHAT" && s.cfg.CollectionPaused {
		return out, httpx.E("PAYMENT_COLLECTION_PAUSED", "暂时停止新收款，已有交易仍会继续处理", 409)
	}
	if !s.channelAllowed(channel) {
		return out, httpx.E("PAYMENT_CHANNEL_UNAVAILABLE", "当前环境不支持此支付通道，请切换联调环境", 409)
	}
	tx, e := database.BeginTx(ctx, s.db)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var total, owner int64
	var status string
	var required bool
	var expires *time.Time
	e = tx.QueryRowContext(ctx, `SELECT customer_id,total_amount,status,payment_required,payment_expires_at FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&owner, &total, &status, &required, &expires)
	if e == sql.ErrNoRows || e == nil && owner != p.SubjectID {
		return out, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	}
	if e != nil {
		return out, e
	}
	if !required || total <= 0 || status != "PENDING_CONFIRMATION" || expires == nil || !expires.After(time.Now()) {
		return out, httpx.E("ORDER_NOT_PAYABLE", "订单无需支付、已过期或当前不可支付", 409)
	}
	existing, e := scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payment_order WHERE org_id=$1 AND order_id=$2 AND status IN ('PENDING','SUCCESS')`, p.OrgID, orderID))
	if e != nil && e != sql.ErrNoRows {
		return out, e
	}
	if e == nil {
		if existing.Channel != channel || existing.Status != "PENDING" {
			return out, httpx.E("PAYMENT_CONFLICT", "已有支付交易，请查询支付结果", 409)
		}
		out.Payment = existing
	} else {
		out.Payment, e = scanPayment(tx.QueryRowContext(ctx, `INSERT INTO payment_order(org_id,order_id,customer_id,channel,out_trade_no,amount,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING `+paymentColumns, p.OrgID, orderID, p.SubjectID, channel, randomID("P"), total, expires))
		if e != nil {
			return out, e
		}
	}
	if e = tx.Commit(); e != nil {
		return out, e
	}
	if channel == "WECHAT" {
		var id int64
		fmt.Sscan(out.Payment.ID, &id)
		until := time.Now().Add(2 * time.Minute)
		if expires.Before(until) {
			until = *expires
		}
		out.Ticket = signTicket(ticket{id, p.OrgID, p.SubjectID, total, s.cfg.AppID, until.Unix()}, s.cfg.TicketSecret)
		out.CloudEnv = s.cfg.CloudEnv
		out.CloudFunction = s.cfg.CloudFunction
	}
	return out, nil
}

// Authorize is reachable only by the authenticated bridge. OpenID comes from our login binding, never from request headers.
func (s *Service) Authorize(ctx context.Context, value string) (map[string]any, error) {
	if !s.cfg.WechatEnabled || s.cfg.CollectionPaused {
		return nil, httpx.E("PAYMENT_DISABLED", "真实支付未启用", 409)
	}
	t, e := readTicket(value, s.cfg.TicketSecret)
	if e != nil {
		return nil, e
	}
	tx, e := database.BeginTx(ctx, s.db)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var orderID int64
	if e = tx.QueryRowContext(ctx, `SELECT order_id FROM payment_order WHERE id=$1 AND org_id=$2 AND customer_id=$3 AND channel='WECHAT'`, t.PaymentID, t.OrgID, t.CustomerID).Scan(&orderID); e != nil {
		return nil, httpx.E("PAYMENT_NOT_FOUND", "支付单不存在", 404)
	}
	var orderStatus string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE id=$1 AND org_id=$2 FOR UPDATE`, orderID, t.OrgID).Scan(&orderStatus); e != nil {
		return nil, e
	}
	var trade, openid, status, prepayID string
	var amount int64
	var expires time.Time
	var lease *time.Time
	e = tx.QueryRowContext(ctx, `SELECT p.out_trade_no,p.amount,p.status,p.expires_at,COALESCE(p.prepay_id,''),p.prepay_lease_until,i.openid FROM payment_order p JOIN customer_wechat_identity i ON i.org_id=p.org_id AND i.customer_id=p.customer_id AND i.appid=$4 WHERE p.id=$1 AND p.org_id=$2 AND p.customer_id=$3 AND p.channel='WECHAT' ORDER BY i.id DESC LIMIT 1 FOR UPDATE OF p`, t.PaymentID, t.OrgID, t.CustomerID, s.cfg.AppID).Scan(&trade, &amount, &status, &expires, &prepayID, &lease, &openid)
	if e != nil {
		return nil, httpx.E("PAYMENT_IDENTITY_INVALID", "支付身份不存在", 409)
	}
	if t.AppID != s.cfg.AppID || t.Amount != amount || status != "PENDING" || orderStatus != "PENDING_CONFIRMATION" || !expires.After(time.Now()) {
		return nil, httpx.E("PAYMENT_NOT_PAYABLE", "支付单已失效", 409)
	}
	if prepayID == "" {
		if lease != nil && lease.After(time.Now()) {
			return nil, httpx.E("PAYMENT_PREPARING", "预支付处理中，请稍后查询或重试", 409)
		}
		if _, e = tx.ExecContext(ctx, `UPDATE payment_order SET prepay_lease_until=now()+interval '30 seconds' WHERE id=$1`, t.PaymentID); e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return map[string]any{"out_trade_no": trade, "amount": map[string]any{"total": amount, "currency": "CNY"}, "description": "灵狸护家服务订单", "time_expire": expires.UTC().Format(time.RFC3339), "payer": map[string]string{"openid": openid}, "prepay_id": prepayID}, nil
}

func (s *Service) SavePrepay(ctx context.Context, value, id string) error {
	t, e := readTicket(value, s.cfg.TicketSecret)
	if e != nil {
		return e
	}
	if !s.cfg.WechatEnabled || t.AppID != s.cfg.AppID || len(id) == 0 || len(id) > 128 {
		return httpx.E("PAYMENT_PREPAY_INVALID", "预支付标识无效", 400)
	}
	result, e := s.db.ExecContext(ctx, `UPDATE payment_order SET prepay_id=$1,prepay_lease_until=NULL WHERE id=$2 AND org_id=$3 AND customer_id=$4 AND channel='WECHAT' AND amount=$5 AND status='PENDING' AND (prepay_id IS NULL OR prepay_id=$1)`, id, t.PaymentID, t.OrgID, t.CustomerID, t.Amount)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return httpx.E("PAYMENT_PREPAY_CONFLICT", "预支付记录冲突，请查询订单", 409)
	}
	return nil
}

type Event struct {
	Missing       bool   `json:"missing,omitempty"`
	Kind          string `json:"kind"`
	Channel       string `json:"channel"`
	OutTradeNo    string `json:"outTradeNo"`
	OutRefundNo   string `json:"outRefundNo,omitempty"`
	TransactionID string `json:"transactionId"`
	RefundID      string `json:"refundId,omitempty"`
	Amount        int64  `json:"amount"`
	Total         int64  `json:"total"`
	Currency      string `json:"currency"`
	AppID         string `json:"appId"`
	MerchantID    string `json:"merchantId"`
	Status        string `json:"status"`
}

func validateEvent(ev Event, c config.Payment, env string) error {
	if ev.Channel == "MOCK" {
		if env != "local" || !c.MockEnabled {
			return httpx.E("PAYMENT_CHANNEL_UNAVAILABLE", "模拟支付未启用", 403)
		}
	} else if ev.Channel != "WECHAT" || !c.WechatEnabled || ev.AppID != c.AppID || ev.MerchantID != c.MerchantID {
		return httpx.E("PAYMENT_EVENT_INVALID", "支付事件身份不符", 403)
	}
	if ev.Currency != "CNY" || ev.Amount <= 0 || ev.TransactionID == "" || ev.OutTradeNo == "" || ev.Kind != "PAYMENT" && ev.Kind != "REFUND" {
		return httpx.E("PAYMENT_EVENT_INVALID", "支付事件不完整", 400)
	}
	if ev.Kind == "PAYMENT" && ev.Status != "SUCCESS" && ev.Status != "CLOSED" {
		return httpx.E("PAYMENT_EVENT_INVALID", "非终态支付事件", 400)
	}
	if ev.Kind == "REFUND" && (ev.OutRefundNo == "" || ev.RefundID == "" || (ev.Status != "SUCCESS" && ev.Status != "ABNORMAL" && ev.Status != "CLOSED")) {
		return httpx.E("PAYMENT_EVENT_INVALID", "退款事件不完整", 400)
	}
	return nil
}
func (s *Service) Apply(ctx context.Context, ev Event) error {
	if e := validateEvent(ev, s.cfg, s.env); e != nil {
		return e
	}
	tx, e := database.BeginTx(ctx, s.db)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var org, orderID int64
	if e = tx.QueryRowContext(ctx, `SELECT org_id,order_id FROM payment_order WHERE out_trade_no=$1 AND channel=$2`, ev.OutTradeNo, ev.Channel).Scan(&org, &orderID); e != nil {
		return httpx.E("PAYMENT_NOT_FOUND", "支付单不存在", 404)
	}
	var orderStatus string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE id=$1 AND org_id=$2 FOR UPDATE`, orderID, org).Scan(&orderStatus); e != nil {
		return e
	}
	pay, e := scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payment_order WHERE out_trade_no=$1 FOR UPDATE`, ev.OutTradeNo))
	if e != nil {
		return e
	}
	if ev.Channel == "MOCK" && ev.Kind == "PAYMENT" && ev.Status == "SUCCESS" && pay.Status != "SUCCESS" && (orderStatus == "CANCELLED" || !pay.ExpiresAt.After(time.Now())) {
		return httpx.E("PAYMENT_NOT_PAYABLE", "模拟订单已取消或过期", 409)
	}
	if pay.Amount != ev.Total || ev.Kind == "PAYMENT" && pay.Amount != ev.Amount || ev.Currency != "CNY" {
		return httpx.E("PAYMENT_AMOUNT_MISMATCH", "支付金额不匹配", 409)
	}
	if pay.TransactionID != "" && pay.TransactionID != ev.TransactionID {
		return httpx.E("PAYMENT_TRANSACTION_MISMATCH", "交易号不符", 409)
	}
	key := ev.Channel + ":" + ev.Kind + ":" + ev.OutTradeNo + ":" + ev.OutRefundNo + ":" + ev.Status
	raw, _ := json.Marshal(ev)
	digest := sha256.Sum256(raw)
	var old []byte
	e = tx.QueryRowContext(ctx, `SELECT payload_hash FROM payment_event WHERE event_key=$1`, key).Scan(&old)
	if e == nil {
		if !equalBytes(old, digest[:]) {
			return httpx.E("PAYMENT_EVENT_CONFLICT", "相同事件内容不一致", 409)
		}
		return tx.Commit()
	}
	if e != sql.ErrNoRows {
		return e
	}
	if ev.Kind == "PAYMENT" {
		if ev.Status == "SUCCESS" && pay.Status != "SUCCESS" {
			if pay.Status != "PENDING" && pay.Status != "CLOSED" {
				return httpx.E("PAYMENT_STATUS_CONFLICT", "支付状态冲突", 409)
			}
			if _, e = tx.ExecContext(ctx, `UPDATE payment_order SET status='SUCCESS',transaction_id=$1,paid_at=now(),updated_at=now(),last_error=NULL WHERE id=$2::bigint`, ev.TransactionID, pay.ID); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE customer_order SET paid_amount=paid_amount+$1,version=version+1 WHERE id=$2`, pay.Amount, orderID); e != nil {
				return e
			}
			if orderStatus == "CANCELLED" {
				if _, e = tx.ExecContext(ctx, `UPDATE payment_order SET last_error='LATE_PAYMENT_REQUIRES_REFUND' WHERE id=$1::bigint`, pay.ID); e != nil {
					return e
				}
			}
		} else if ev.Status == "CLOSED" && pay.Status == "PENDING" {
			if _, e = tx.ExecContext(ctx, `UPDATE payment_order SET status='CLOSED',updated_at=now() WHERE id=$1::bigint`, pay.ID); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE customer_order SET status='CANCELLED',cancel_reason='支付超时或交易已关闭',cancelled_at=now(),version=version+1 WHERE id=$1 AND status='PENDING_CONFIRMATION' AND paid_amount=0`, orderID); e != nil {
				return e
			}
		}
	} else {
		var rid, amount int64
		var state string
		if e = tx.QueryRowContext(ctx, `SELECT id,amount,status FROM payment_refund WHERE payment_id=$1::bigint AND out_refund_no=$2 FOR UPDATE`, pay.ID, ev.OutRefundNo).Scan(&rid, &amount, &state); e != nil {
			return e
		}
		if amount != ev.Amount || pay.Status != "SUCCESS" {
			return httpx.E("REFUND_AMOUNT_MISMATCH", "退款金额或支付状态不符", 409)
		}
		if state == "SUCCESS" && ev.Status != "SUCCESS" {
			return tx.Commit()
		}
		if _, e = tx.ExecContext(ctx, `UPDATE payment_refund SET status=$1,refund_id=$2,completed_at=CASE WHEN $1='SUCCESS' THEN now() ELSE completed_at END,last_error=NULL WHERE id=$3`, ev.Status, ev.RefundID, rid); e != nil {
			return e
		}
		if ev.Status == "SUCCESS" && state != "SUCCESS" {
			if _, e = tx.ExecContext(ctx, `UPDATE customer_order SET refunded_amount=refunded_amount+$1,version=version+1 WHERE id=$2`, amount, orderID); e != nil {
				return e
			}
		}
		if ev.Status == "SUCCESS" {
			if _, e = tx.ExecContext(ctx, `UPDATE payment_order SET last_error=NULL WHERE id=$1::bigint AND last_error='LATE_PAYMENT_REQUIRES_REFUND' AND EXISTS(SELECT 1 FROM customer_order WHERE id=$2 AND refunded_amount>=paid_amount)`, pay.ID, orderID); e != nil {
				return e
			}
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO payment_event(event_key,channel,payload_hash) VALUES($1,$2,$3)`, key, ev.Channel, digest[:]); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_log(org_id,action_code,resource_type,resource_id,detail_json) VALUES($1,$2,'PAYMENT',$3,$4)`, org, "payment."+ev.Kind+"."+ev.Status, pay.ID, raw); e != nil {
		return e
	}
	return tx.Commit()
}
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (s *Service) Mock(ctx context.Context, p auth.Principal, orderID int64, outcome string) error {
	if !s.mockAllowed() {
		return httpx.E("PAYMENT_CHANNEL_UNAVAILABLE", "模拟支付未启用", 403)
	}
	overview, e := s.Overview(ctx, p, orderID, false)
	if e != nil {
		return e
	}
	if len(overview.Payments) == 0 {
		return httpx.E("PAYMENT_NOT_FOUND", "请先创建支付单", 409)
	}
	pay := overview.Payments[0]
	if pay.Channel != "MOCK" {
		return httpx.E("PAYMENT_CHANNEL_MISMATCH", "不能模拟真实交易", 403)
	}
	if outcome == "CANCEL" || outcome == "FAIL" || outcome == "PENDING" {
		return nil
	}
	if outcome != "SUCCESS" {
		return httpx.E("VALIDATION_ERROR", "无效模拟结果", 400)
	}
	if !pay.ExpiresAt.After(time.Now()) && pay.Status != "SUCCESS" {
		return httpx.E("PAYMENT_EXPIRED", "支付单已过期", 409)
	}
	return s.Apply(ctx, Event{Kind: "PAYMENT", Channel: "MOCK", OutTradeNo: pay.OutTradeNo, TransactionID: "MOCK_" + pay.OutTradeNo, Amount: pay.Amount, Total: pay.Amount, Currency: "CNY", Status: "SUCCESS"})
}

func (s *Service) Refund(ctx context.Context, p auth.Principal, orderID, amount int64, reason, key string) (Refund, error) {
	var out Refund
	if amount <= 0 || len(reason) == 0 || len([]rune(reason)) > 512 || len(key) == 0 || len(key) > 128 {
		return out, httpx.E("VALIDATION_ERROR", "请填写有效退款金额、原因和幂等键", 400)
	}
	tx, e := database.BeginTx(ctx, s.db)
	if e != nil {
		return out, e
	}
	defer tx.Rollback()
	var status string
	if e = tx.QueryRowContext(ctx, `SELECT status FROM customer_order WHERE org_id=$1 AND id=$2 FOR UPDATE`, p.OrgID, orderID).Scan(&status); e != nil {
		return out, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	}
	pay, e := scanPayment(tx.QueryRowContext(ctx, `SELECT `+paymentColumns+` FROM payment_order WHERE org_id=$1 AND order_id=$2 AND status='SUCCESS' FOR UPDATE`, p.OrgID, orderID))
	if e != nil {
		return out, httpx.E("PAYMENT_NOT_FOUND", "没有成功支付记录", 409)
	}
	if !s.channelAllowed(pay.Channel) {
		return out, httpx.E("PAYMENT_CHANNEL_UNAVAILABLE", "支付通道未配置", 409)
	}
	var savedPayment string
	var operator int64
	e = tx.QueryRowContext(ctx, `SELECT id::text,out_refund_no,amount,reason,status,created_at,payment_id::text,operator_id FROM payment_refund WHERE org_id=$1 AND idempotency_key=$2`, p.OrgID, key).Scan(&out.ID, &out.OutRefundNo, &out.Amount, &out.Reason, &out.Status, &out.CreatedAt, &savedPayment, &operator)
	if e == nil {
		if savedPayment != pay.ID || out.Amount != amount || out.Reason != reason || operator != p.AdminUserID {
			return out, httpx.E("IDEMPOTENCY_CONFLICT", "幂等键已用于其他退款", 409)
		}
		return out, tx.Commit()
	}
	if e != sql.ErrNoRows {
		return out, e
	}
	var reserved int64
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM payment_refund WHERE payment_id=$1::bigint AND status IN ('PROCESSING','SUCCESS','ABNORMAL')`, pay.ID).Scan(&reserved); e != nil {
		return out, e
	}
	if amount > pay.Amount-reserved {
		return out, httpx.E("REFUND_EXCEEDS_BALANCE", "退款金额超过可退余额", 409)
	}
	e = tx.QueryRowContext(ctx, `INSERT INTO payment_refund(org_id,payment_id,out_refund_no,amount,reason,operator_id,idempotency_key) VALUES($1,$2::bigint,$3,$4,$5,$6,$7) RETURNING id::text,out_refund_no,amount,reason,status,created_at`, p.OrgID, pay.ID, randomID("R"), amount, reason, p.AdminUserID, key).Scan(&out.ID, &out.OutRefundNo, &out.Amount, &out.Reason, &out.Status, &out.CreatedAt)
	if e != nil {
		return out, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO audit_log(org_id,principal_id,action_code,resource_type,resource_id,detail_json) VALUES($1,$2,'payment.refund.request','ORDER',$3,jsonb_build_object('refundNo',$4::text,'amount',$5::bigint,'reason',$6::text))`, p.OrgID, p.AdminUserID, fmt.Sprint(orderID), out.OutRefundNo, amount, reason); e != nil {
		return out, e
	}
	if e = tx.Commit(); e != nil {
		return out, e
	}
	// Accepted requests remain durable and are retried by the reconciliation job.
	_ = s.processRefund(ctx, pay, out)
	return out, nil
}
func (s *Service) processRefund(ctx context.Context, p Payment, r Refund) error {
	ev := Event{Kind: "REFUND", Channel: p.Channel, OutTradeNo: p.OutTradeNo, OutRefundNo: r.OutRefundNo, TransactionID: p.TransactionID, RefundID: "MOCK_" + r.OutRefundNo, Amount: r.Amount, Total: p.Amount, Currency: "CNY", Status: "SUCCESS"}
	if p.Channel == "WECHAT" {
		var e error
		ev, e = s.bridge.call(ctx, "refund", map[string]any{"out_trade_no": p.OutTradeNo, "out_refund_no": r.OutRefundNo, "reason": r.Reason, "amount": map[string]any{"refund": r.Amount, "total": p.Amount, "currency": "CNY"}})
		if e != nil {
			return e
		}
		if ev.Status == "PROCESSING" {
			return nil
		}
	}
	return s.Apply(ctx, ev)
}

// Reconcile performs channel queries before closing or recovering transactions.
func (s *Service) Reconcile(ctx context.Context) error {
	rows, e := s.db.QueryContext(ctx, `SELECT `+paymentColumns+` FROM payment_order WHERE status IN ('PENDING','SUCCESS') AND next_check_at<=now() ORDER BY next_check_at LIMIT 30`)
	if e != nil {
		return e
	}
	var payments []Payment
	for rows.Next() {
		p, e := scanPayment(rows)
		if e != nil {
			rows.Close()
			return e
		}
		payments = append(payments, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	var errs []error
	for _, p := range payments {
		if !s.channelAllowed(p.Channel) {
			continue
		}
		var err error
		if p.Channel == "MOCK" {
			if p.Status == "PENDING" && time.Now().After(p.ExpiresAt) {
				err = s.Apply(ctx, Event{Kind: "PAYMENT", Channel: "MOCK", OutTradeNo: p.OutTradeNo, TransactionID: "MOCK_" + p.OutTradeNo, Amount: p.Amount, Total: p.Amount, Currency: "CNY", Status: "CLOSED"})
			}
		} else {
			var ev Event
			ev, err = s.bridge.call(ctx, "query", map[string]any{"out_trade_no": p.OutTradeNo, "expired": p.Status == "PENDING" && time.Now().After(p.ExpiresAt)})
			if err == nil && p.Status == "SUCCESS" && ev.Status != "SUCCESS" {
				err = fmt.Errorf("channel reconciliation mismatch for payment %s", p.ID)
			}
			if err == nil && p.Status == "PENDING" && ev.Missing && ev.OutTradeNo == p.OutTradeNo && time.Now().After(p.ExpiresAt.Add(5*time.Minute)) {
				ev = Event{Kind: "PAYMENT", Channel: "WECHAT", OutTradeNo: p.OutTradeNo, TransactionID: "CLOSED_" + p.OutTradeNo, Amount: p.Amount, Total: p.Amount, Currency: "CNY", AppID: s.cfg.AppID, MerchantID: s.cfg.MerchantID, Status: "CLOSED"}
			}
			if err == nil && (ev.Status == "SUCCESS" || ev.Status == "CLOSED") {
				err = s.Apply(ctx, ev)
			}
		}
		summary := ""
		if err != nil {
			summary = "CHANNEL_CHECK_FAILED"
			errs = append(errs, err)
		}
		_, e = s.db.ExecContext(ctx, `UPDATE payment_order SET next_check_at=CASE WHEN status='SUCCESS' AND $1='' THEN now()+interval '24 hours' ELSE now()+make_interval(secs=>LEAST(3600,30*power(2,LEAST(check_count,7)))::double precision) END,check_count=check_count+1,last_error=CASE WHEN last_error='LATE_PAYMENT_REQUIRES_REFUND' THEN last_error ELSE NULLIF($1,'') END WHERE id=$2::bigint`, summary, p.ID)
		if e != nil {
			errs = append(errs, e)
		}
	}
	rr, e := s.db.QueryContext(ctx, `SELECT r.id::text,r.out_refund_no,r.amount,r.reason,r.status,r.created_at,p.id::text,p.order_id::text,p.channel,p.out_trade_no,COALESCE(p.transaction_id,''),p.amount,p.status,p.expires_at,p.paid_at,COALESCE(p.last_error,'') FROM payment_refund r JOIN payment_order p ON p.id=r.payment_id WHERE r.status IN ('PROCESSING','ABNORMAL','SUCCESS') AND r.next_check_at<=now() ORDER BY r.next_check_at LIMIT 30`)
	if e != nil {
		return e
	}
	type pending struct {
		p Payment
		r Refund
	}
	var refunds []pending
	for rr.Next() {
		var x pending
		if e = rr.Scan(&x.r.ID, &x.r.OutRefundNo, &x.r.Amount, &x.r.Reason, &x.r.Status, &x.r.CreatedAt, &x.p.ID, &x.p.OrderID, &x.p.Channel, &x.p.OutTradeNo, &x.p.TransactionID, &x.p.Amount, &x.p.Status, &x.p.ExpiresAt, &x.p.PaidAt, &x.p.LastError); e != nil {
			rr.Close()
			return e
		}
		refunds = append(refunds, x)
	}
	e = rr.Err()
	rr.Close()
	if e != nil {
		return e
	}
	for _, x := range refunds {
		if !s.channelAllowed(x.p.Channel) {
			continue
		}
		err := s.processRefund(ctx, x.p, x.r)
		summary := ""
		if err != nil {
			summary = "REFUND_CHECK_FAILED"
			errs = append(errs, err)
		}
		_, e = s.db.ExecContext(ctx, `UPDATE payment_refund SET next_check_at=CASE WHEN status='SUCCESS' AND $1='' THEN now()+interval '24 hours' ELSE now()+make_interval(secs=>LEAST(3600,30*power(2,LEAST(check_count,7)))::double precision) END,check_count=check_count+1,last_error=NULLIF($1,'') WHERE id=$2::bigint`, summary, x.r.ID)
		if e != nil {
			errs = append(errs, e)
		}
	}
	// Orders abandoned before any payment attempt still expire locally.
	_, e = s.db.ExecContext(ctx, `UPDATE customer_order o SET status='CANCELLED',cancelled_at=now(),cancel_reason='支付超时',version=version+1 WHERE payment_required AND paid_amount=0 AND status='PENDING_CONFIRMATION' AND payment_expires_at<now() AND NOT EXISTS(SELECT 1 FROM payment_order p WHERE p.order_id=o.id AND p.status IN ('PENDING','SUCCESS'))`)
	if e != nil {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}
