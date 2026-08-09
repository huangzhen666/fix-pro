package cart

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net/http"
	"strings"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db} }

type FaultMedia struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
	Name      string `json:"name"`
	URL       string `json:"url"`
}
type Item struct {
	ID               string       `json:"id"`
	SKUID            string       `json:"skuId"`
	SKUVersion       int          `json:"skuVersion"`
	Name             string       `json:"name"`
	CoverImageURL    string       `json:"coverImageUrl"`
	UnitPrice        int64        `json:"unitPrice"`
	Unit             string       `json:"unit"`
	Quantity         int          `json:"quantity"`
	Subtotal         int64        `json:"subtotal"`
	FaultDescription *string      `json:"faultDescription"`
	FaultMedia       []FaultMedia `json:"faultMedia"`
	FaultComplete    bool         `json:"faultComplete"`
}
type View struct {
	Items       []Item `json:"items"`
	ItemCount   int    `json:"itemCount"`
	TotalAmount int64  `json:"totalAmount"`
}
type FaultWrite struct {
	FaultDescription string   `json:"faultDescription"`
	MediaIDs         []string `json:"mediaIds"`
}

func parse(v string) (int64, error) {
	var n int64
	_, e := fmt.Sscan(v, &n)
	if e != nil || n <= 0 {
		return 0, httpx.E("VALIDATION_ERROR", "ID 格式错误", 400)
	}
	return n, nil
}
func (s *Service) find(ctx context.Context, p auth.Principal) (int64, error) {
	var n int64
	e := s.db.QueryRowContext(ctx, `SELECT id FROM shopping_cart WHERE org_id=$1 AND customer_id=$2`, p.OrgID, p.SubjectID).Scan(&n)
	return n, e
}
func (s *Service) media(ctx context.Context, item int64) ([]FaultMedia, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT m.id,m.media_type,m.original_name FROM shopping_cart_item_media cm JOIN media_asset m ON m.id=cm.media_id AND m.org_id=cm.org_id WHERE cm.org_id=1 AND cm.cart_item_id=$1 ORDER BY cm.sort_order`, item)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []FaultMedia{}
	for rows.Next() {
		var x FaultMedia
		var n int64
		if e = rows.Scan(&n, &x.MediaType, &x.Name); e != nil {
			return nil, e
		}
		x.ID = fmt.Sprint(n)
		x.URL = "/api/v1/mini/media/" + x.ID + "/content"
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) Get(ctx context.Context, p auth.Principal) (View, error) {
	cart, e := s.find(ctx, p)
	if e == sql.ErrNoRows {
		return View{Items: []Item{}}, nil
	}
	if e != nil {
		return View{}, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT ci.id,ci.sku_id,ci.sku_version,ci.quantity,ci.unit_price,ci.fault_description,v.snapshot_json FROM shopping_cart_item ci JOIN service_sku_version v ON v.org_id=ci.org_id AND v.sku_id=ci.sku_id AND v.version_no=ci.sku_version WHERE ci.org_id=$1 AND ci.cart_id=$2 ORDER BY ci.created_at`, p.OrgID, cart)
	if e != nil {
		return View{}, e
	}
	defer rows.Close()
	out := View{Items: []Item{}}
	for rows.Next() {
		var x Item
		var item, sku int64
		var fault sql.NullString
		var raw []byte
		var snap catalog.PublishedSKU
		if e = rows.Scan(&item, &sku, &x.SKUVersion, &x.Quantity, &x.UnitPrice, &fault, &raw); e != nil {
			return out, e
		}
		if e = json.Unmarshal(raw, &snap); e != nil {
			return out, e
		}
		x.ID, x.SKUID = fmt.Sprint(item), fmt.Sprint(sku)
		x.Name, x.CoverImageURL, x.Unit = snap.Name, snap.CoverImageURL, snap.Unit
		x.Subtotal = x.UnitPrice * int64(x.Quantity)
		if fault.Valid {
			x.FaultDescription = &fault.String
		}
		x.FaultMedia, e = s.media(ctx, item)
		if e != nil {
			return out, e
		}
		x.FaultComplete = fault.Valid && len([]rune(strings.TrimSpace(fault.String))) >= 5 && len(x.FaultMedia) > 0
		out.Items = append(out.Items, x)
		out.ItemCount += x.Quantity
		out.TotalAmount += x.Subtotal
	}
	return out, rows.Err()
}
func (s *Service) current(ctx context.Context, n int64) (catalog.PublishedSKU, error) {
	var raw []byte
	var x catalog.PublishedSKU
	e := s.db.QueryRowContext(ctx, `SELECT v.snapshot_json FROM service_sku s JOIN service_sku_version v ON v.org_id=s.org_id AND v.sku_id=s.id AND v.version_no=s.current_published_version WHERE s.org_id=1 AND s.id=$1 AND s.status='PUBLISHED'`, n).Scan(&raw)
	if e == sql.ErrNoRows {
		return x, httpx.E("SKU_NOT_AVAILABLE", "服务不可购买", 404)
	}
	if e != nil {
		return x, e
	}
	e = json.Unmarshal(raw, &x)
	return x, e
}
func (s *Service) ensure(ctx context.Context, p auth.Principal) (int64, error) {
	n, e := s.find(ctx, p)
	if e == nil {
		return n, nil
	}
	if e != sql.ErrNoRows {
		return 0, e
	}
	var id int64
	e = s.db.QueryRowContext(ctx, `INSERT INTO shopping_cart(org_id,customer_id) VALUES($1,$2) ON CONFLICT (org_id,customer_id) DO NOTHING RETURNING id`, p.OrgID, p.SubjectID).Scan(&id)
	if e == sql.ErrNoRows {
		return s.find(ctx, p)
	}
	return id, e
}
func (s *Service) Add(ctx context.Context, p auth.Principal, sku int64, q int) (View, error) {
	if q < 1 || q > 99 {
		return View{}, httpx.E("VALIDATION_ERROR", "数量必须为 1-99", 400)
	}
	x, e := s.current(ctx, sku)
	if e != nil {
		return View{}, e
	}
	cart, e := s.ensure(ctx, p)
	if e != nil {
		return View{}, e
	}
	var itemID int64
	e = s.db.QueryRowContext(ctx, `INSERT INTO shopping_cart_item(org_id,cart_id,sku_id,sku_version,quantity,unit_price) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (org_id,cart_id,sku_id,sku_version) DO UPDATE SET quantity=shopping_cart_item.quantity+EXCLUDED.quantity WHERE shopping_cart_item.quantity+EXCLUDED.quantity<=99 RETURNING id`, p.OrgID, cart, sku, x.PublishedVersion, q, x.Price).Scan(&itemID)
	if e == sql.ErrNoRows {
		return View{}, httpx.E("VALIDATION_ERROR", "购物车数量不能超过 99", 400)
	}
	if e != nil {
		return View{}, e
	}
	return s.Get(ctx, p)
}
func (s *Service) assert(ctx context.Context, p auth.Principal, item int64) error {
	var n int
	e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shopping_cart_item ci JOIN shopping_cart sc ON sc.id=ci.cart_id WHERE ci.id=$1 AND ci.org_id=$2 AND sc.customer_id=$3`, item, p.OrgID, p.SubjectID).Scan(&n)
	if e != nil {
		return e
	}
	if n == 0 {
		return httpx.E("CART_ITEM_NOT_FOUND", "购物车项目不存在", 404)
	}
	return nil
}
func (s *Service) Quantity(ctx context.Context, p auth.Principal, item int64, q int) (View, error) {
	if q < 1 || q > 99 {
		return View{}, httpx.E("VALIDATION_ERROR", "数量必须为 1-99", 400)
	}
	if e := s.assert(ctx, p, item); e != nil {
		return View{}, e
	}
	_, e := s.db.ExecContext(ctx, `UPDATE shopping_cart_item SET quantity=$1 WHERE id=$2 AND org_id=$3`, q, item, p.OrgID)
	if e != nil {
		return View{}, e
	}
	return s.Get(ctx, p)
}
func (s *Service) Fault(ctx context.Context, p auth.Principal, item int64, w FaultWrite) (View, error) {
	desc := strings.TrimSpace(w.FaultDescription)
	if len([]rune(desc)) < 5 || len([]rune(desc)) > 500 {
		return View{}, httpx.E("FAULT_DESCRIPTION_REQUIRED", "故障描述需为 5-500 字", 400)
	}
	unique := []int64{}
	seen := map[int64]bool{}
	for _, v := range w.MediaIDs {
		n, e := parse(v)
		if e != nil {
			return View{}, e
		}
		if !seen[n] {
			unique = append(unique, n)
			seen[n] = true
		}
	}
	if len(unique) == 0 {
		return View{}, httpx.E("FAULT_MEDIA_REQUIRED", "请至少上传一张故障图片或一个视频", 400)
	}
	if len(unique) > 8 {
		return View{}, httpx.E("MEDIA_COUNT_EXCEEDED", "故障媒体最多 8 个", 400)
	}
	if e := s.assert(ctx, p, item); e != nil {
		return View{}, e
	}
	images, videos := 0, 0
	for _, n := range unique {
		var kind string
		e := s.db.QueryRowContext(ctx, `SELECT media_type FROM media_asset WHERE id=$1 AND org_id=$2 AND owner_type='CUSTOMER' AND owner_id=$3 AND purpose='FAULT_EVIDENCE' AND status='READY'`, n, p.OrgID, p.SubjectID).Scan(&kind)
		if e == sql.ErrNoRows {
			return View{}, httpx.E("MEDIA_ACCESS_DENIED", "故障媒体不属于当前客户", 403)
		}
		if e != nil {
			return View{}, e
		}
		if kind == "IMAGE" {
			images++
		} else {
			videos++
		}
	}
	if images > 6 || videos > 2 {
		return View{}, httpx.E("MEDIA_COUNT_EXCEEDED", "最多 6 张图片和 2 个视频", 400)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return View{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE shopping_cart_item SET fault_description=$1 WHERE id=$2 AND org_id=$3`, desc, item, p.OrgID); e != nil {
		return View{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM shopping_cart_item_media WHERE org_id=$1 AND cart_item_id=$2`, p.OrgID, item); e != nil {
		return View{}, e
	}
	for i, n := range unique {
		if _, e = tx.ExecContext(ctx, `INSERT INTO shopping_cart_item_media(org_id,cart_item_id,media_id,sort_order) VALUES($1,$2,$3,$4)`, p.OrgID, item, n, i); e != nil {
			return View{}, e
		}
	}
	if e = tx.Commit(); e != nil {
		return View{}, e
	}
	return s.Get(ctx, p)
}
func (s *Service) Delete(ctx context.Context, p auth.Principal, item int64) (View, error) {
	if e := s.assert(ctx, p, item); e != nil {
		return View{}, e
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return View{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `DELETE FROM shopping_cart_item_media WHERE org_id=$1 AND cart_item_id=$2`, p.OrgID, item); e != nil {
		return View{}, e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM shopping_cart_item WHERE org_id=$1 AND id=$2`, p.OrgID, item); e != nil {
		return View{}, e
	}
	if e = tx.Commit(); e != nil {
		return View{}, e
	}
	return s.Get(ctx, p)
}

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	v, e := h.s.Get(r.Context(), p)
	respond(w, r, v, e)
}
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var b struct {
		SKUID    string `json:"skuId"`
		Quantity int    `json:"quantity"`
	}
	if e := httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	n, e := parse(b.SKUID)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Add(r.Context(), p, n, b.Quantity)
	respond(w, r, v, e)
}
func (h *Handler) Quantity(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var b struct {
		Quantity int `json:"quantity"`
	}
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Quantity(r.Context(), p, n, b.Quantity)
	respond(w, r, v, e)
}
func (h *Handler) Fault(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var b FaultWrite
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Fault(r.Context(), p, n, b)
	respond(w, r, v, e)
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Delete(r.Context(), p, n)
	respond(w, r, v, e)
}
func respond(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		httpx.Failure(w, r, e)
	} else {
		httpx.Success(w, r, v)
	}
}
