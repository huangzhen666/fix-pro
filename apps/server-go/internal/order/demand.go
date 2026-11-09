package order

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type DemandInput struct {
	ItemID            string   `json:"itemId"`
	DemandDescription string   `json:"demandDescription"`
	MediaIDs          []string `json:"mediaIds"`
}
type DemandItem struct {
	DemandInput
	Name        string  `json:"name"`
	DemandMedia []Media `json:"demandMedia"`
}
type OrderDemand struct {
	OrderID        string       `json:"orderId"`
	DemandVersion  int          `json:"demandVersion"`
	Editable       bool         `json:"editable"`
	ReadOnlyReason string       `json:"readOnlyReason"`
	Items          []DemandItem `json:"items"`
}
type DemandWrite struct {
	DemandVersion int           `json:"demandVersion"`
	Items         []DemandInput `json:"items"`
}

func readDemand(ctx context.Context, tx *sql.Tx, p auth.Principal, id int64, write bool) (OrderDemand, error) {
	out := OrderDemand{OrderID: fmt.Sprint(id), Items: []DemandItem{}}
	lock := " FOR SHARE"
	if write {
		lock = " FOR UPDATE"
	}
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status,demand_version FROM customer_order WHERE org_id=$1 AND customer_id=$2 AND id=$3`+lock, p.OrgID, p.SubjectID, id).Scan(&status, &out.DemandVersion)
	if err == sql.ErrNoRows {
		return out, httpx.E("ORDER_NOT_FOUND", "订单不存在", 404)
	}
	if err != nil {
		return out, err
	}
	out.Editable = status == "PENDING_CONFIRMATION" || status == "FULFILLING"
	if !out.Editable {
		out.ReadOnlyReason = "当前订单状态不支持修改备注"
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,sku_name_snapshot,COALESCE(demand_description,'') FROM order_item WHERE org_id=$1 AND order_id=$2 ORDER BY id`, p.OrgID, id)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item DemandItem
		var n int64
		if err = rows.Scan(&n, &item.Name, &item.DemandDescription); err != nil {
			rows.Close()
			return out, err
		}
		item.ItemID = fmt.Sprint(n)
		item.MediaIDs = []string{}
		item.DemandMedia = []Media{}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		rows, err = tx.QueryContext(ctx, `SELECT m.id,m.media_type,m.original_name FROM order_item_media om JOIN media_asset m ON m.id=om.media_id AND m.org_id=om.org_id WHERE om.org_id=$1 AND om.order_item_id=$2 ORDER BY om.sort_order,om.id`, p.OrgID, out.Items[i].ItemID)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var m Media
			var mid int64
			if err = rows.Scan(&mid, &m.MediaType, &m.Name); err != nil {
				rows.Close()
				return out, err
			}
			m.ID = fmt.Sprint(mid)
			m.URL = "/api/v1/mini/media/" + m.ID + "/content"
			out.Items[i].MediaIDs = append(out.Items[i].MediaIDs, m.ID)
			out.Items[i].DemandMedia = append(out.Items[i].DemandMedia, m)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) GetDemand(ctx context.Context, p auth.Principal, id int64) (OrderDemand, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OrderDemand{}, err
	}
	defer tx.Rollback()
	out, err := readDemand(ctx, tx, p, id, false)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) UpdateDemand(ctx context.Context, p auth.Principal, id int64, key string, w DemandWrite) (OrderDemand, error) {
	var out OrderDemand
	if key == "" || len(key) > 80 {
		return out, httpx.E("VALIDATION_ERROR", "缺少有效的保存标识", 400)
	}
	key = fmt.Sprintf("demand:%d:%s", id, key)
	raw, _ := json.Marshal(struct {
		OrderID int64
		Body    DemandWrite
	}{id, w})
	hash := sha256.Sum256(raw)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	before, err := readDemand(ctx, tx, p, id, true)
	if err != nil {
		return out, err
	}
	var oldHash []byte
	var response []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_record WHERE org_id=$1 AND principal_type='CUSTOMER' AND principal_id=$2 AND idempotency_key=$3`, p.OrgID, p.SubjectID, key).Scan(&oldHash, &response)
	if err == nil {
		if string(oldHash) != string(hash[:]) {
			return out, httpx.E("IDEMPOTENCY_CONFLICT", "保存标识已用于不同内容", 409)
		}
		if err = json.Unmarshal(response, &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return out, err
	}
	if !before.Editable {
		return out, httpx.E("ORDER_DEMAND_READ_ONLY", before.ReadOnlyReason, 409)
	}
	if w.DemandVersion != before.DemandVersion {
		return out, httpx.E("ORDER_DEMAND_VERSION_CONFLICT", "备注已更新，请重新加载后核对；本次输入已保留", 409)
	}
	if len(w.Items) != len(before.Items) {
		return out, httpx.E("VALIDATION_ERROR", "请提交该订单全部服务的备注", 400)
	}
	expected := map[string]bool{}
	for _, it := range before.Items {
		expected[it.ItemID] = true
	}
	mediaSet := map[int64]bool{}
	for _, it := range before.Items {
		for _, v := range it.MediaIDs {
			n, _ := strconv.ParseInt(v, 10, 64)
			mediaSet[n] = true
		}
	}
	for i := range w.Items {
		it := &w.Items[i]
		if !expected[it.ItemID] {
			return out, httpx.E("VALIDATION_ERROR", "订单服务不匹配或重复", 400)
		}
		delete(expected, it.ItemID)
		it.DemandDescription = strings.TrimSpace(it.DemandDescription)
		if err = catalog.CheckDemand(it.DemandDescription, len(it.MediaIDs)); err != nil {
			return out, err
		}
		seen := map[int64]bool{}
		for _, v := range it.MediaIDs {
			n, e := strconv.ParseInt(v, 10, 64)
			if e != nil || n <= 0 || seen[n] {
				return out, httpx.E("VALIDATION_ERROR", "现场资料标识无效或重复", 400)
			}
			seen[n] = true
			mediaSet[n] = true
		}
	}
	ids := make([]int64, 0, len(mediaSet))
	for n := range mediaSet {
		ids = append(ids, n)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, n := range ids {
		var ok bool
		err = tx.QueryRowContext(ctx, `SELECT owner_type='CUSTOMER' AND owner_id=$1 AND purpose='DEMAND_EVIDENCE' AND status='READY' FROM media_asset WHERE org_id=$2 AND id=$3 FOR UPDATE`, p.SubjectID, p.OrgID, n).Scan(&ok)
		if err == sql.ErrNoRows || err == nil && !ok {
			return out, httpx.E("VALIDATION_ERROR", "现场资料不可用，请重新选择", 400)
		}
		if err != nil {
			return out, err
		}
	}
	for _, it := range w.Items {
		if _, err = tx.ExecContext(ctx, `UPDATE order_item SET demand_description=$1 WHERE org_id=$2 AND order_id=$3 AND id=$4`, it.DemandDescription, p.OrgID, id, it.ItemID); err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM order_item_media WHERE org_id=$1 AND order_item_id=$2`, p.OrgID, it.ItemID); err != nil {
			return out, err
		}
		for i, mid := range it.MediaIDs {
			if _, err = tx.ExecContext(ctx, `INSERT INTO order_item_media(org_id,order_item_id,media_id,media_type,sort_order) SELECT $1,$2,id,media_type,$4 FROM media_asset WHERE org_id=$1 AND id=$3`, p.OrgID, it.ItemID, mid, i); err != nil {
				return out, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE customer_order SET demand_version=demand_version+1 WHERE org_id=$1 AND id=$2`, p.OrgID, id); err != nil {
		return out, err
	}
	out, err = readDemand(ctx, tx, p, id, true)
	if err != nil {
		return out, err
	}
	b, _ := json.Marshal(before.Items)
	a, _ := json.Marshal(out.Items)
	var history int64
	if err = tx.QueryRowContext(ctx, `INSERT INTO order_demand_history(org_id,order_id,customer_id,demand_version,before_json,after_json) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, p.OrgID, id, p.SubjectID, out.DemandVersion, b, a).Scan(&history); err != nil {
		return out, err
	}
	for _, mid := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO order_demand_history_media(history_id,media_id) VALUES($1,$2)`, history, mid); err != nil {
			return out, err
		}
	}
	response, _ = json.Marshal(out)
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_record(org_id,principal_type,principal_id,idempotency_key,request_hash,response_code,response_body,expires_at) VALUES($1,'CUSTOMER',$2,$3,$4,'OK',$5,$6)`, p.OrgID, p.SubjectID, key, hash[:], response, time.Now().Add(24*time.Hour)); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (h *Handler) Demand(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	if r.Method == http.MethodGet {
		out, e := h.s.GetDemand(r.Context(), p, id)
		send(w, r, out, e)
		return
	}
	var b DemandWrite
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.s.UpdateDemand(r.Context(), p, id, r.Header.Get("Idempotency-Key"), b)
	send(w, r, out, e)
}
