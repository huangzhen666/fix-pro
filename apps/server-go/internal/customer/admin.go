package customer

import (
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Usage struct {
	OrderCount     int64      `json:"orderCount"`
	CompletedCount int64      `json:"completedCount"`
	CancelledCount int64      `json:"cancelledCount"`
	PaidAmount     int64      `json:"paidAmount"`
	LastOrderAt    *time.Time `json:"lastOrderAt"`
}
type AdminCustomer struct {
	ID             string     `json:"id"`
	DisplayName    string     `json:"displayName"`
	Status         string     `json:"status"`
	MobileMasked   string     `json:"mobileMasked"`
	AvatarURL      *string    `json:"avatarUrl"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastLoginAt    *time.Time `json:"lastLoginAt"`
	LastActivityAt *time.Time `json:"lastActivityAt"`
	Usage          Usage      `json:"usage"`
}

func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	now := time.Now().UTC()
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	local := now.In(zone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	seven := start.AddDate(0, 0, -6)
	thirty := start.AddDate(0, 0, -29)
	var total, today, new7, active, active7, active30 int64
	e := h.s.DB.QueryRowContext(r.Context(), `SELECT count(*),count(*) FILTER(WHERE created_at >=$2 AND created_at<=$5),count(*) FILTER(WHERE created_at >=$3 AND created_at<=$5),count(*) FILTER(WHERE deleted_at IS NULL AND last_activity_at >=$2 AND last_activity_at<=$5),count(*) FILTER(WHERE deleted_at IS NULL AND last_activity_at >=$3 AND last_activity_at<=$5),count(*) FILTER(WHERE deleted_at IS NULL AND last_activity_at >=$4 AND last_activity_at<=$5) FROM customer WHERE org_id=$1 AND source_channel='WECHAT_MINI' AND created_at<=$5`, p.OrgID, start, seven, thirty, now).Scan(&total, &today, &new7, &active, &active7, &active30)
	respond(w, r, map[string]any{"registered": total, "newToday": today, "new7Days": new7, "activeToday": active, "active7Days": active7, "active30Days": active30, "asOf": now, "timezone": "Asia/Shanghai"}, e)
}
func pagination(r *http.Request) (int, int, error) {
	page, size := 1, 20
	var e error
	if v := r.URL.Query().Get("page"); v != "" {
		page, e = strconv.Atoi(v)
		if e != nil || page < 1 {
			return 0, 0, httpx.E("VALIDATION_ERROR", "分页参数无效", 400)
		}
	}
	if v := r.URL.Query().Get("pageSize"); v != "" {
		size, e = strconv.Atoi(v)
		if e != nil || size < 1 || size > 100 {
			return 0, 0, httpx.E("VALIDATION_ERROR", "每页应为 1 至 100 条", 400)
		}
	}
	if page > 1000000 {
		return 0, 0, httpx.E("VALIDATION_ERROR", "页码超出范围", 400)
	}
	return page, size, nil
}

const aggregate = `COALESCE(o.n,0),COALESCE(o.completed,0),COALESCE(o.cancelled,0),COALESCE(o.paid,0),o.last_order`
const lateral = ` LEFT JOIN LATERAL(SELECT count(*) n,count(*) FILTER(WHERE status='COMPLETED') completed,count(*) FILTER(WHERE status='CANCELLED') cancelled,COALESCE(sum(paid_amount),0) paid,max(created_at) last_order FROM customer_order WHERE org_id=c.org_id AND customer_id=c.id) o ON true `
const customerCols = `c.id,c.display_name,CASE WHEN c.deleted_at IS NOT NULL THEN 'DELETED' ELSE c.status END,c.mobile_cipher,c.avatar_media_id,c.created_at,c.last_login_at,c.last_activity_at,`

func (h *Handler) scanAdmin(row interface{ Scan(...any) error }) (AdminCustomer, error) {
	var c AdminCustomer
	var id int64
	var encrypted []byte
	var avatar sql.NullInt64
	e := row.Scan(&id, &c.DisplayName, &c.Status, &encrypted, &avatar, &c.CreatedAt, &c.LastLoginAt, &c.LastActivityAt, &c.Usage.OrderCount, &c.Usage.CompletedCount, &c.Usage.CancelledCount, &c.Usage.PaidAmount, &c.Usage.LastOrderAt)
	if e != nil {
		return c, e
	}
	c.ID = fmt.Sprint(id)
	if len(encrypted) > 0 && h.s.Codec != nil {
		v, e := h.s.Codec.Decrypt(encrypted)
		if e != nil {
			return c, e
		}
		c.MobileMasked = MaskPhone(v)
	}
	if avatar.Valid {
		u := "/api/v1/admin/customers/" + c.ID + "/avatar/content?media=" + fmt.Sprint(avatar.Int64)
		c.AvatarURL = &u
	}
	return c, nil
}
func (h *Handler) Customers(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	page, size, e := pagination(r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	q := r.URL.Query()
	args := []any{p.OrgID}
	where := `c.org_id=$1 AND c.source_channel='WECHAT_MINI'`
	add := func(clause string, v any) { args = append(args, v); where += " AND " + fmt.Sprintf(clause, len(args)) }
	status := q.Get("status")
	if status == "DELETED" {
		where += " AND c.deleted_at IS NOT NULL"
	} else {
		where += " AND c.deleted_at IS NULL"
		if status != "" {
			if status != "ACTIVE" && status != "DISABLED" {
				respond(w, r, nil, httpx.E("VALIDATION_ERROR", "状态无效", 400))
				return
			}
			add("c.status=$%d", status)
		}
	}
	if id := q.Get("customerId"); id != "" {
		n, e := strconv.ParseInt(id, 10, 64)
		if e != nil || n <= 0 {
			respond(w, r, nil, httpx.E("VALIDATION_ERROR", "用户编号无效", 400))
			return
		}
		add("c.id=$%d", n)
	}
	if name := strings.TrimSpace(q.Get("name")); name != "" {
		if len(name) > 128 {
			respond(w, r, nil, httpx.E("VALIDATION_ERROR", "昵称搜索过长", 400))
			return
		}
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(name)
		add("c.display_name ILIKE $%d", "%"+escaped+"%")
	}
	if mobile := q.Get("mobile"); mobile != "" {
		n, e := NormalizePhone(mobile)
		if e != nil || h.s.Codec == nil {
			respond(w, r, nil, httpx.E("VALIDATION_ERROR", "请输入完整大陆手机号", 400))
			return
		}
		add("c.mobile_hash=$%d", h.s.Codec.Hash(n))
	}
	for _, k := range []string{"registeredFrom", "registeredTo"} {
		if v := q.Get(k); v != "" {
			t, e := time.ParseInLocation("2006-01-02", v, time.FixedZone("Asia/Shanghai", 8*3600))
			if e != nil {
				respond(w, r, nil, httpx.E("VALIDATION_ERROR", "日期格式无效", 400))
				return
			}
			if k == "registeredFrom" {
				add("c.created_at >= $%d", t)
			} else {
				add("c.created_at < $%d", t.AddDate(0, 0, 1))
			}
		}
	}
	if v := q.Get("hasOrders"); v != "" {
		if v != "true" && v != "false" {
			respond(w, r, nil, httpx.E("VALIDATION_ERROR", "订单筛选无效", 400))
			return
		}
		op := ""
		if v == "false" {
			op = "NOT "
		}
		where += " AND " + op + "EXISTS(SELECT 1 FROM customer_order co WHERE co.org_id=c.org_id AND co.customer_id=c.id)"
	}
	var total int64
	e = h.s.DB.QueryRowContext(r.Context(), "SELECT count(*) FROM customer c WHERE "+where, args...).Scan(&total)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	sorting := map[string]string{"": "c.created_at DESC,c.id DESC", "registered": "c.created_at DESC,c.id DESC", "active": "c.last_activity_at DESC NULLS LAST,c.id DESC", "orders": "(SELECT count(*) FROM customer_order z WHERE z.org_id=c.org_id AND z.customer_id=c.id) DESC,c.id DESC"}
	order, ok := sorting[q.Get("sort")]
	if !ok {
		respond(w, r, nil, httpx.E("VALIDATION_ERROR", "排序无效", 400))
		return
	}
	args = append(args, size, (page-1)*size)
	query := fmt.Sprintf(`WITH selected AS(SELECT c.* FROM customer c WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d) SELECT %s %s FROM selected c %s ORDER BY %s`, where, order, len(args)-1, len(args), customerCols, aggregate, lateral, order)
	rows, e := h.s.DB.QueryContext(r.Context(), query, args...)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	defer rows.Close()
	items := []AdminCustomer{}
	for rows.Next() {
		c, e := h.scanAdmin(rows)
		if e != nil {
			respond(w, r, nil, e)
			return
		}
		items = append(items, c)
	}
	respond(w, r, map[string]any{"items": items, "total": total, "page": page, "pageSize": size}, rows.Err())
}
func (h *Handler) AdminDetail(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	row := h.s.DB.QueryRowContext(r.Context(), "SELECT "+customerCols+aggregate+" FROM customer c "+lateral+" WHERE c.org_id=$1 AND c.id=$2 AND c.source_channel='WECHAT_MINI'", p.OrgID, id)
	out, e := h.scanAdmin(row)
	if e == sql.ErrNoRows {
		e = httpx.E("CUSTOMER_NOT_FOUND", "用户不存在", 404)
	}
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var addresses, cart int64
	e = h.s.DB.QueryRowContext(r.Context(), `SELECT (SELECT count(*) FROM customer_address WHERE org_id=$1 AND customer_id=$2),(SELECT count(*) FROM shopping_cart_item ci JOIN shopping_cart sc ON sc.org_id=ci.org_id AND sc.id=ci.cart_id WHERE sc.org_id=$1 AND sc.customer_id=$2)`, p.OrgID, id).Scan(&addresses, &cart)
	respond(w, r, map[string]any{"customer": out, "addressCount": addresses, "cartItemCount": cart}, e)
}
func (h *Handler) AdminOrders(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	page, size, e := pagination(r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var exists bool
	e = h.s.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM customer WHERE org_id=$1 AND id=$2 AND source_channel='WECHAT_MINI')`, p.OrgID, id).Scan(&exists)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	if !exists {
		respond(w, r, nil, httpx.E("CUSTOMER_NOT_FOUND", "用户不存在", 404))
		return
	}
	var total int64
	e = h.s.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM customer_order WHERE org_id=$1 AND customer_id=$2`, p.OrgID, id).Scan(&total)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	rows, e := h.s.DB.QueryContext(r.Context(), `SELECT id,order_no,status,total_amount,paid_amount,created_at FROM customer_order WHERE org_id=$1 AND customer_id=$2 ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, p.OrgID, id, size, (page-1)*size)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var n, amount, paid int64
		var no, status string
		var at time.Time
		if e = rows.Scan(&n, &no, &status, &amount, &paid, &at); e != nil {
			respond(w, r, nil, e)
			return
		}
		items = append(items, map[string]any{"id": fmt.Sprint(n), "orderNo": no, "status": status, "totalAmount": amount, "paidAmount": paid, "createdAt": at})
	}
	respond(w, r, map[string]any{"items": items, "total": total}, rows.Err())
}
