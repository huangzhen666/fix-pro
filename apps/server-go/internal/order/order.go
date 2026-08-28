package order

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"math"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db} }

type Write struct {
	ContactName     string `json:"contactName"`
	ContactMobile   string `json:"contactMobile"`
	ServiceAddress  string `json:"serviceAddress"`
	AppointmentDate string `json:"appointmentDate"`
	AppointmentSlot string `json:"appointmentSlot"`
}
type Result struct {
	ID          string    `json:"id"`
	OrderNo     string    `json:"orderNo"`
	Status      string    `json:"status"`
	TotalAmount int64     `json:"totalAmount"`
	CreatedAt   time.Time `json:"createdAt"`
}
type Summary struct {
	ID            string    `json:"id"`
	OrderNo       string    `json:"orderNo"`
	Status        string    `json:"status"`
	CancelReason  string    `json:"cancelReason,omitempty"`
	ContactName   string    `json:"contactName"`
	ContactMobile string    `json:"contactMobile"`
	TotalAmount   int64     `json:"totalAmount"`
	ItemCount     int       `json:"itemCount"`
	CreatedAt     time.Time `json:"createdAt"`
	Version       int       `json:"version"`
}
type Page struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
type Base struct {
	ID              string     `json:"id"`
	OrderNo         string     `json:"orderNo"`
	CustomerID      string     `json:"customerId"`
	Status          string     `json:"status"`
	CancelReason    string     `json:"cancelReason,omitempty"`
	ContactName     string     `json:"contactName"`
	ContactMobile   string     `json:"contactMobile"`
	ServiceAddress  string     `json:"serviceAddress"`
	TotalAmount     int64      `json:"totalAmount"`
	ItemCount       int        `json:"itemCount"`
	Version         int        `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	AppointmentAt   *time.Time `json:"appointmentAt,omitempty"`
	AppointmentSlot string     `json:"appointmentSlot,omitempty"`
}
type Media struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	Name      string `json:"name"`
	URL       string `json:"url"`
}
type ItemView struct {
	ID                  string  `json:"id"`
	SKUCode             string  `json:"skuCode"`
	SKUName             string  `json:"skuName"`
	SKUVersion          int     `json:"skuVersion"`
	Unit                string  `json:"unit"`
	ServiceScope        string  `json:"serviceScope"`
	Exclusions          string  `json:"exclusions"`
	WarrantyDescription string  `json:"warrantyDescription"`
	CoverImageURL       string  `json:"coverImageUrl"`
	FaultDescription    string  `json:"faultDescription"`
	UnitPrice           int64   `json:"unitPrice"`
	Quantity            int     `json:"quantity"`
	Subtotal            int64   `json:"subtotal"`
	FaultMedia          []Media `json:"faultMedia"`
}
type Detail struct {
	Order Base       `json:"order"`
	Items []ItemView `json:"items"`
}
type RepeatResult struct {
	ItemsCopied int `json:"itemsCopied"`
}
type checkout struct {
	id, sku           int64
	version, quantity int
	price             int64
	fault, status     string
	current           sql.NullInt64
	snap, currentSnap catalog.PublishedSKU
}

func samePublishedSKU(a, b catalog.PublishedSKU) bool {
	a.PublishedVersion = 0
	b.PublishedVersion = 0
	return reflect.DeepEqual(a, b)
}

var mobile = regexp.MustCompile(`^1\d{10}$`)

func validate(w Write) error {
	if len([]rune(strings.TrimSpace(w.ContactName))) < 2 || len([]rune(w.ContactName)) > 64 {
		return httpx.E("VALIDATION_ERROR", "联系人需为 2-64 字", 400)
	}
	if !mobile.MatchString(w.ContactMobile) {
		return httpx.E("INVALID_CONTACT_MOBILE", "手机号格式错误", 400)
	}
	if len([]rune(strings.TrimSpace(w.ServiceAddress))) < 5 || len([]rune(w.ServiceAddress)) > 255 {
		return httpx.E("VALIDATION_ERROR", "服务地址需为 5-255 字", 400)
	}
	if !validSlot(w.AppointmentSlot) {
		return httpx.E("APPOINTMENT_SLOT_INVALID", "请选择有效的预约时间段", 400)
	}
	date, err := time.ParseInLocation("2006-01-02", w.AppointmentDate, time.Local)
	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if err != nil || date.Before(today.AddDate(0, 0, 1)) || date.After(today.AddDate(0, 0, 30)) {
		return httpx.E("APPOINTMENT_DATE_INVALID", "预约日期必须为明天起30天内", 400)
	}
	return nil
}
func validSlot(slot string) bool {
	switch slot {
	case "08:00", "10:00", "12:00", "14:00", "16:00", "18:00", "20:00":
		return true
	}
	return false
}
func (s *Service) Create(ctx context.Context, p auth.Principal, key string, w Write) (Result, error) {
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return Result{}, httpx.E("VALIDATION_ERROR", "缺少有效 Idempotency-Key", 400)
	}
	if e := validate(w); e != nil {
		return Result{}, e
	}
	date, _ := time.ParseInLocation("2006-01-02", w.AppointmentDate, time.Local)
	hour := 0
	fmt.Sscanf(w.AppointmentSlot, "%d:00", &hour)
	appointmentAt := time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, time.Local).UTC()
	body, _ := json.Marshal(w)
	hash := sha256.Sum256(body)
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return Result{}, e
	}
	defer tx.Rollback()
	var placeholderID int64
	e = tx.QueryRowContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'CUSTOMER',$2,$3,$4,'PROCESSING',NULL,$5) ON CONFLICT (org_id,principal_type,principal_id,idempotency_key) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID, key, hash[:], time.Now().UTC().Add(24*time.Hour)).Scan(&placeholderID)
	if e == sql.ErrNoRows {
		var old []byte
		var raw sql.NullString
		e = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='CUSTOMER' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&old, &raw)
		if e != nil {
			return Result{}, e
		}
		if len(old) != len(hash) || subtle.ConstantTimeCompare(old, hash[:]) != 1 {
			return Result{}, httpx.E("ORDER_SUBMIT_DUPLICATED", "幂等键已用于不同请求", 409)
		}
		if !raw.Valid {
			return Result{}, httpx.E("ORDER_SUBMIT_DUPLICATED", "订单正在提交", 409)
		}
		var out Result
		if e = json.Unmarshal([]byte(raw.String), &out); e != nil {
			return out, e
		}
		return out, nil
	}
	if e != nil {
		return Result{}, e
	}
	var cart int64
	e = tx.QueryRowContext(ctx, `SELECT id FROM shopping_cart WHERE org_id=$1 AND customer_id=$2 FOR UPDATE`, p.OrgID, p.SubjectID).Scan(&cart)
	if e == sql.ErrNoRows {
		return Result{}, httpx.E("CART_EMPTY", "购物车为空", 400)
	}
	if e != nil {
		return Result{}, e
	}
	rows, e := tx.QueryContext(ctx, `SELECT ci.id,ci.sku_id,ci.sku_version,ci.quantity,ci.unit_price,COALESCE(ci.fault_description,''),v.snapshot_json,s.status,s.current_published_version,cv.snapshot_json FROM shopping_cart_item ci JOIN service_sku_version v ON v.org_id=ci.org_id AND v.sku_id=ci.sku_id AND v.version_no=ci.sku_version JOIN service_sku s ON s.id=ci.sku_id AND s.org_id=ci.org_id LEFT JOIN service_sku_version cv ON cv.org_id=s.org_id AND cv.sku_id=s.id AND cv.version_no=s.current_published_version WHERE ci.org_id=$1 AND ci.cart_id=$2 FOR UPDATE OF ci, v, s`, p.OrgID, cart)
	if e != nil {
		return Result{}, e
	}
	items := []checkout{}
	for rows.Next() {
		var x checkout
		var raw, currentRaw []byte
		if e = rows.Scan(&x.id, &x.sku, &x.version, &x.quantity, &x.price, &x.fault, &raw, &x.status, &x.current, &currentRaw); e != nil {
			rows.Close()
			return Result{}, e
		}
		if e = json.Unmarshal(raw, &x.snap); e != nil {
			rows.Close()
			return Result{}, e
		}
		if len(currentRaw) > 0 {
			if e = json.Unmarshal(currentRaw, &x.currentSnap); e != nil {
				rows.Close()
				return Result{}, e
			}
		}
		items = append(items, x)
	}
	rows.Close()
	if len(items) == 0 {
		return Result{}, httpx.E("CART_EMPTY", "购物车为空", 400)
	}
	var total int64
	count := 0
	for _, x := range items {
		var media int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM shopping_cart_item_media cim JOIN media_asset m ON m.id=cim.media_id AND m.org_id=cim.org_id WHERE cim.org_id=$1 AND cim.cart_item_id=$2 AND m.status='READY'`, p.OrgID, x.id).Scan(&media); e != nil {
			return Result{}, e
		}
		if x.status != "PUBLISHED" || !x.current.Valid || x.currentSnap.ID == "" || x.price != x.currentSnap.Price {
			return Result{}, httpx.E("CART_SKU_CHANGED", "服务价格或版本已变化，请刷新购物车", 409)
		}
		if int(x.current.Int64) != x.version {
			if !samePublishedSKU(x.snap, x.currentSnap) {
				return Result{}, httpx.E("CART_SKU_CHANGED", "服务价格或版本已变化，请刷新购物车", 409)
			}
			x.version = int(x.current.Int64)
			x.snap = x.currentSnap
			x.price = x.currentSnap.Price
		}
		if x.quantity <= 0 || x.price > math.MaxInt64/int64(x.quantity) {
			return Result{}, httpx.E("VALIDATION_ERROR", "订单金额超出范围", 400)
		}
		subtotal := x.price * int64(x.quantity)
		if total > math.MaxInt64-subtotal {
			return Result{}, httpx.E("VALIDATION_ERROR", "订单金额超出范围", 400)
		}
		total += subtotal
		count += x.quantity
	}
	orderNo := fmt.Sprintf("FP%s%06d", time.Now().UTC().Format("20060102150405"), time.Now().UnixNano()%1000000)
	var orderID int64
	e = tx.QueryRowContext(ctx, `INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count,appointment_at,appointment_slot) VALUES($1,$2,$3,$4,$5,$6,'REPAIR',$7,$8,0,$9,$10,$11) RETURNING id`, p.OrgID, orderNo, p.SubjectID, strings.TrimSpace(w.ContactName), w.ContactMobile, strings.TrimSpace(w.ServiceAddress), fulfillment.OrderPendingConfirmation, total, count, appointmentAt, w.AppointmentSlot).Scan(&orderID)
	if e != nil {
		return Result{}, e
	}
	for _, x := range items {
		cover := int64(0)
		fmt.Sscan(x.snap.CoverImageURL[strings.LastIndex(x.snap.CoverImageURL, "/")+1:], &cover)
		var itemID int64
		e = tx.QueryRowContext(ctx, `INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,sku_cover_media_id_snapshot,fault_description,unit_price,quantity,subtotal_amount) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`, p.OrgID, orderID, x.sku, x.version, x.snap.SKUCode, x.snap.Name, x.snap.Unit, x.snap.ServiceScope, x.snap.Exclusions, x.snap.WarrantyDescription, cover, strings.TrimSpace(x.fault), x.price, x.quantity, x.price*int64(x.quantity)).Scan(&itemID)
		if e != nil {
			return Result{}, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO order_item_media(org_id,order_item_id,media_id,media_type,sort_order) SELECT cim.org_id,$1,cim.media_id,m.media_type,cim.sort_order FROM shopping_cart_item_media cim JOIN media_asset m ON m.id=cim.media_id AND m.org_id=cim.org_id WHERE cim.org_id=$2 AND cim.cart_item_id=$3`, itemID, p.OrgID, x.id); e != nil {
			return Result{}, e
		}
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO order_status_history(org_id,order_id,from_status,to_status,event_code,operator_type,operator_id,operator_name) VALUES($1,$2,NULL,$3,'ORDER_CREATED','CUSTOMER',$4,$5)`, p.OrgID, orderID, fulfillment.OrderPendingConfirmation, p.SubjectID, p.Name); e != nil {
		return Result{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM shopping_cart_item_media cim USING shopping_cart_item ci WHERE ci.id=cim.cart_item_id AND ci.org_id=cim.org_id AND ci.org_id=$1 AND ci.cart_id=$2`, p.OrgID, cart); e != nil {
		return Result{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM shopping_cart_item WHERE org_id=$1 AND cart_id=$2`, p.OrgID, cart); e != nil {
		return Result{}, e
	}
	out := Result{fmt.Sprint(orderID), orderNo, fulfillment.OrderPendingConfirmation, total, time.Now().UTC()}
	raw, _ := json.Marshal(out)
	if _, e = tx.ExecContext(ctx, `UPDATE idempotency_record SET response_code='OK',response_body=$1 WHERE org_id=$2 AND principal_type='CUSTOMER' AND principal_id=$3 AND idempotency_key=$4`, raw, p.OrgID, p.SubjectID, key); e != nil {
		return Result{}, e
	}
	if e = tx.Commit(); e != nil {
		return Result{}, e
	}
	return out, nil
}

type repeatItem struct {
	orderItemID, skuID int64
	quantity           int
	fault              string
	status             string
	currentVersion     sql.NullInt64
	snap               catalog.PublishedSKU
}

func (s *Service) Repeat(ctx context.Context, p auth.Principal, orderID int64) (RepeatResult, error) {
	if p.Role != "CUSTOMER" {
		return RepeatResult{}, httpx.E("FORBIDDEN", "无权重新下单", 403)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return RepeatResult{}, e
	}
	defer tx.Rollback()

	var orderStatus, cancelReason string
	e = tx.QueryRowContext(ctx, `SELECT status,COALESCE(cancel_reason,'') FROM customer_order WHERE org_id=$1 AND id=$2 AND customer_id=$3 FOR SHARE`, p.OrgID, orderID, p.SubjectID).Scan(&orderStatus, &cancelReason)
	if e == sql.ErrNoRows {
		return RepeatResult{}, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	}
	if e != nil {
		return RepeatResult{}, e
	}
	if orderStatus != fulfillment.OrderCancelled || strings.TrimSpace(cancelReason) == "" {
		return RepeatResult{}, httpx.E("ORDER_STATUS_CONFLICT", "只有商家打回的订单可以重新下单", 409)
	}

	rows, e := tx.QueryContext(ctx, `SELECT oi.id,oi.sku_id,oi.quantity,COALESCE(oi.fault_description,''),s.status,s.current_published_version,cv.snapshot_json FROM order_item oi JOIN service_sku s ON s.org_id=oi.org_id AND s.id=oi.sku_id LEFT JOIN service_sku_version cv ON cv.org_id=s.org_id AND cv.sku_id=s.id AND cv.version_no=s.current_published_version WHERE oi.org_id=$1 AND oi.order_id=$2 ORDER BY oi.id`, p.OrgID, orderID)
	if e != nil {
		return RepeatResult{}, e
	}
	items := []repeatItem{}
	for rows.Next() {
		var item repeatItem
		var raw []byte
		if e = rows.Scan(&item.orderItemID, &item.skuID, &item.quantity, &item.fault, &item.status, &item.currentVersion, &raw); e != nil {
			rows.Close()
			return RepeatResult{}, e
		}
		if item.quantity < 1 || item.quantity > 99 {
			rows.Close()
			return RepeatResult{}, httpx.E("VALIDATION_ERROR", "订单服务数量不合法", 400)
		}
		if item.status != "PUBLISHED" || !item.currentVersion.Valid || len(raw) == 0 {
			rows.Close()
			return RepeatResult{}, httpx.E("SKU_NOT_AVAILABLE", "原订单中的服务已下架，暂时不能重新下单", 409)
		}
		if e = json.Unmarshal(raw, &item.snap); e != nil {
			rows.Close()
			return RepeatResult{}, e
		}
		if item.snap.ID == "" {
			rows.Close()
			return RepeatResult{}, httpx.E("SKU_NOT_AVAILABLE", "原订单中的服务已下架，暂时不能重新下单", 409)
		}
		items = append(items, item)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return RepeatResult{}, e
	}
	if len(items) == 0 {
		return RepeatResult{}, httpx.E("ORDER_EMPTY", "订单没有可复制的服务", 400)
	}

	var cartID int64
	e = tx.QueryRowContext(ctx, `INSERT INTO shopping_cart(org_id,customer_id) VALUES($1,$2) ON CONFLICT (org_id,customer_id) DO UPDATE SET updated_at=shopping_cart.updated_at RETURNING id`, p.OrgID, p.SubjectID).Scan(&cartID)
	if e != nil {
		return RepeatResult{}, e
	}
	for _, item := range items {
		var cartItemID int64
		e = tx.QueryRowContext(ctx, `INSERT INTO shopping_cart_item(org_id,cart_id,sku_id,sku_version,quantity,unit_price,fault_description) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')) ON CONFLICT (org_id,cart_id,sku_id,sku_version) DO UPDATE SET quantity=shopping_cart_item.quantity+EXCLUDED.quantity,fault_description=CASE WHEN NULLIF(BTRIM(shopping_cart_item.fault_description),'') IS NULL THEN EXCLUDED.fault_description ELSE shopping_cart_item.fault_description END,updated_at=CURRENT_TIMESTAMP(3) WHERE shopping_cart_item.quantity+EXCLUDED.quantity<=99 RETURNING id`, p.OrgID, cartID, item.skuID, item.currentVersion.Int64, item.quantity, item.snap.Price, strings.TrimSpace(item.fault)).Scan(&cartItemID)
		if e == sql.ErrNoRows {
			return RepeatResult{}, httpx.E("CART_QUANTITY_EXCEEDED", "购物车数量不能超过99", 400)
		}
		if e != nil {
			return RepeatResult{}, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO shopping_cart_item_media(org_id,cart_item_id,media_id,sort_order) SELECT $1,$2,oim.media_id,oim.sort_order FROM order_item_media oim WHERE oim.org_id=$1 AND oim.order_item_id=$3 ON CONFLICT (org_id,cart_item_id,media_id) DO NOTHING`, p.OrgID, cartItemID, item.orderItemID); e != nil {
			return RepeatResult{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return RepeatResult{}, e
	}
	return RepeatResult{ItemsCopied: len(items)}, nil
}

func (s *Service) List(ctx context.Context, key, status, contact, createdFrom, createdTo string, page, size int) (Page, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := "%" + strings.TrimSpace(key) + "%"
	c := "%" + strings.TrimSpace(contact) + "%"
	where := `org_id=1 AND ($1='' OR order_no ILIKE $1 OR contact_name ILIKE $1 OR contact_mobile ILIKE $1) AND ($2='' OR status=$2) AND ($3='' OR contact_name ILIKE $3 OR contact_mobile ILIKE $3) AND ($4='' OR created_at >= $4::date) AND ($5='' OR created_at < ($5::date + INTERVAL '1 day'))`
	out := Page{Items: []Summary{}, Page: page, PageSize: size}
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM customer_order WHERE `+where, q, status, c, createdFrom, createdTo).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id,order_no,status,COALESCE(cancel_reason,''),contact_name,contact_mobile,total_amount,item_count,created_at,version FROM customer_order WHERE `+where+` ORDER BY created_at DESC LIMIT $6 OFFSET $7`, q, status, c, createdFrom, createdTo, size, (page-1)*size)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var x Summary
		var n int64
		if e = rows.Scan(&n, &x.OrderNo, &x.Status, &x.CancelReason, &x.ContactName, &x.ContactMobile, &x.TotalAmount, &x.ItemCount, &x.CreatedAt, &x.Version); e != nil {
			return out, e
		}
		x.ID = fmt.Sprint(n)
		if len(x.ContactMobile) >= 11 {
			x.ContactMobile = x.ContactMobile[:3] + "****" + x.ContactMobile[7:]
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}
func (s *Service) media(ctx context.Context, item int64) ([]Media, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT m.id,m.media_type,m.original_name FROM order_item_media om JOIN media_asset m ON m.id=om.media_id AND m.org_id=om.org_id WHERE om.org_id=1 AND om.order_item_id=$1 ORDER BY om.sort_order`, item)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Media{}
	for rows.Next() {
		var x Media
		var n int64
		if e = rows.Scan(&n, &x.MediaType, &x.Name); e != nil {
			return out, e
		}
		x.ID = fmt.Sprint(n)
		x.URL = "/api/v1/admin/media/" + x.ID + "/content"
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) Detail(ctx context.Context, n int64) (Detail, error) {
	var out Detail
	var id, customer int64
	e := s.db.QueryRowContext(ctx, `SELECT id,order_no,customer_id,status,COALESCE(cancel_reason,''),contact_name,contact_mobile,service_address,total_amount,item_count,version,created_at,appointment_at,COALESCE(appointment_slot,'') FROM customer_order WHERE org_id=1 AND id=$1`, n).Scan(&id, &out.Order.OrderNo, &customer, &out.Order.Status, &out.Order.CancelReason, &out.Order.ContactName, &out.Order.ContactMobile, &out.Order.ServiceAddress, &out.Order.TotalAmount, &out.Order.ItemCount, &out.Order.Version, &out.Order.CreatedAt, &out.Order.AppointmentAt, &out.Order.AppointmentSlot)
	if e == sql.ErrNoRows {
		return out, httpx.E("RESOURCE_NOT_FOUND", "订单不存在", 404)
	}
	if e != nil {
		return out, e
	}
	out.Order.ID, out.Order.CustomerID = fmt.Sprint(id), fmt.Sprint(customer)
	rows, e := s.db.QueryContext(ctx, `SELECT id,sku_code_snapshot,sku_name_snapshot,sku_version,unit_snapshot,service_scope_snapshot,exclusions_snapshot,warranty_snapshot,sku_cover_media_id_snapshot,fault_description,unit_price,quantity,subtotal_amount FROM order_item WHERE org_id=1 AND order_id=$1 ORDER BY id`, n)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	out.Items = []ItemView{}
	for rows.Next() {
		var x ItemView
		var item, cover int64
		if e = rows.Scan(&item, &x.SKUCode, &x.SKUName, &x.SKUVersion, &x.Unit, &x.ServiceScope, &x.Exclusions, &x.WarrantyDescription, &cover, &x.FaultDescription, &x.UnitPrice, &x.Quantity, &x.Subtotal); e != nil {
			return out, e
		}
		x.ID = fmt.Sprint(item)
		x.CoverImageURL = "/api/v1/public/media/" + fmt.Sprint(cover)
		x.FaultMedia, e = s.media(ctx, item)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func ints(r *http.Request, k string, d int) int {
	var n int
	fmt.Sscan(r.URL.Query().Get(k), &n)
	if n == 0 {
		return d
	}
	return n
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var b Write
	if e := httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Create(r.Context(), p, r.Header.Get("Idempotency-Key"), b)
	send(w, r, v, e)
}
func (h *Handler) Repeat(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Repeat(r.Context(), p, n)
	send(w, r, v, e)
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	v, e := h.s.List(r.Context(), r.URL.Query().Get("keyword"), r.URL.Query().Get("status"), r.URL.Query().Get("contact"), r.URL.Query().Get("createdFrom"), r.URL.Query().Get("createdTo"), ints(r, "page", 1), ints(r, "pageSize", 20))
	send(w, r, v, e)
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Detail(r.Context(), n)
	send(w, r, v, e)
}
func send(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		httpx.Failure(w, r, e)
	} else {
		httpx.Success(w, r, v)
	}
}
