package order

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/media"
	"github.com/fixpro/server/internal/platform/auth"
)

type demandFixture struct {
	db                  *sql.DB
	svc                 *Service
	p                   auth.Principal
	order               int64
	items               []string
	mediaID, otherMedia int64
	ms                  *media.Service
}

func newDemandFixture(t *testing.T) demandFixture {
	t.Helper()
	dsn := os.Getenv("ORDER_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable ORDER_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	var name string
	if e = db.QueryRow(`SELECT current_database()`).Scan(&name); e != nil || !strings.HasSuffix(name, "_test") {
		db.Close()
		t.Fatal("disposable database required", e)
	}
	t.Cleanup(func() { db.Close() })
	stamp := fmt.Sprint(time.Now().UnixNano())
	f := demandFixture{db: db, svc: New(db)}
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	var customer int64
	must(db.QueryRow(`INSERT INTO customer(org_id,display_name,source_channel,mobile_cipher,mobile_hash) VALUES(1,'需求测试','TEST',decode('01','hex'),decode(md5($1::text)||md5($1::text),'hex')) RETURNING id`, stamp).Scan(&customer))
	f.p = auth.Principal{OrgID: 1, SubjectID: customer, Role: "CUSTOMER"}
	must(db.QueryRow(`INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,item_count,appointment_slot,appointment_at) VALUES(1,$1,$2,'测试客户','13800138000','汉川市测试地址','SERVICE','PENDING_CONFIRMATION',23600,2,'08:00',now()+interval '1 day') RETURNING id`, "DEMAND-"+stamp, customer).Scan(&f.order))
	for i := 0; i < 2; i++ {
		var id int64
		must(db.QueryRow(`INSERT INTO order_item(org_id,order_id,sku_id,sku_version,sku_code_snapshot,sku_name_snapshot,unit_snapshot,sku_cover_media_id_snapshot,demand_description,unit_price,quantity,subtotal_amount,service_description_snapshot) VALUES(1,$1,100,1,$2,'测试服务','次',0,'',11800,1,11800,'不可修改的成交说明') RETURNING id`, f.order, fmt.Sprintf("SKU-%s-%d", stamp, i)).Scan(&id))
		f.items = append(f.items, fmt.Sprint(id))
	}
	for i := 0; i < 2; i++ {
		var id int64
		owner := customer
		if i == 1 {
			owner = customer + 100000
		}
		must(db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'CUSTOMER',$1,'DEMAND_EVIDENCE','IMAGE','site.png',$2,'image/png',1,'READY') RETURNING id`, owner, fmt.Sprintf("demand-%s-%d.png", stamp, i)).Scan(&id))
		if i == 0 {
			f.mediaID = id
		} else {
			f.otherMedia = id
		}
	}
	root := t.TempDir()
	var key string
	must(db.QueryRow(`SELECT object_key FROM media_asset WHERE id=$1`, f.mediaID).Scan(&key))
	must(os.WriteFile(filepath.Join(root, key), []byte{1}, 0600))
	f.ms, e = media.New(db, root)
	must(e)
	t.Cleanup(func() {
		for _, q := range []string{`DELETE FROM work_order_item WHERE work_order_id IN(SELECT id FROM work_order WHERE order_id=$1)`, `DELETE FROM work_order WHERE order_id=$1`, `DELETE FROM order_demand_history WHERE order_id=$1`, `DELETE FROM order_item_media WHERE order_item_id IN(SELECT id FROM order_item WHERE order_id=$1)`, `DELETE FROM order_item WHERE order_id=$1`, `DELETE FROM customer_order WHERE id=$1`} {
			if _, err := db.Exec(q, f.order); err != nil {
				t.Error(err)
			}
		}
		db.Exec(`DELETE FROM idempotency_record WHERE principal_type='CUSTOMER' AND principal_id=$1`, customer)
		db.Exec(`DELETE FROM media_asset WHERE id IN($1,$2)`, f.mediaID, f.otherMedia)
		db.Exec(`DELETE FROM customer WHERE id=$1`, customer)
	})
	return f
}
func (f demandFixture) write(v int, text string, mediaIDs ...string) DemandWrite {
	return DemandWrite{DemandVersion: v, Items: []DemandInput{{ItemID: f.items[0], DemandDescription: text, MediaIDs: mediaIDs}, {ItemID: f.items[1], DemandDescription: "第二项"}}}
}
func TestOrderDemandLifecycle(t *testing.T) {
	f := newDemandFixture(t)
	ctx := context.Background()
	before, e := f.svc.GetDemand(ctx, f.p, f.order)
	if e != nil || !before.Editable || before.DemandVersion != 0 || len(before.Items) != 2 {
		t.Fatalf("initial %+v %v", before, e)
	}
	w := f.write(0, "先联系我", fmt.Sprint(f.mediaID))
	saved, e := f.svc.UpdateDemand(ctx, f.p, f.order, "first", w)
	if e != nil || saved.DemandVersion != 1 {
		t.Fatalf("save %+v %v", saved, e)
	}
	replay, e := f.svc.UpdateDemand(ctx, f.p, f.order, "first", w)
	if e != nil || replay.DemandVersion != 1 {
		t.Fatalf("replay %v", e)
	}
	w.Items[0].DemandDescription = "不同正文"
	_, e = f.svc.UpdateDemand(ctx, f.p, f.order, "first", w)
	if e == nil {
		t.Fatal("changed payload replay accepted")
	}
	bad := f.write(1, "不得部分保存")
	bad.Items[1].MediaIDs = []string{fmt.Sprint(f.otherMedia)}
	_, e = f.svc.UpdateDemand(ctx, f.p, f.order, "invalid-media", bad)
	if e == nil {
		t.Fatal("foreign media accepted")
	}
	actual, e := f.svc.GetDemand(ctx, f.p, f.order)
	if e != nil || actual.DemandVersion != 1 || actual.Items[0].DemandDescription != "先联系我" {
		t.Fatalf("partial save %+v %v", actual, e)
	}
	for _, p := range []auth.Principal{{OrgID: 2, SubjectID: f.p.SubjectID, Role: "CUSTOMER"}, {OrgID: 1, SubjectID: f.p.SubjectID + 1, Role: "CUSTOMER"}} {
		if _, e = f.svc.GetDemand(ctx, p, f.order); e == nil {
			t.Fatal("foreign order read")
		}
		if _, e = f.svc.UpdateDemand(ctx, p, f.order, "foreign", f.write(1, "攻击")); e == nil {
			t.Fatal("foreign order write")
		}
	}
	if _, e = f.db.Exec(`UPDATE customer_order SET status='FULFILLING' WHERE id=$1`, f.order); e != nil {
		t.Fatal(e)
	}
	var workID int64
	e = f.db.QueryRow(`INSERT INTO work_order(org_id,work_order_no,order_id,assignee_id,status,priority) VALUES(1,$1,$2,999999,'IN_SERVICE','NORMAL') RETURNING id`, fmt.Sprint("WK-", f.order), f.order).Scan(&workID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(`INSERT INTO work_order_item(org_id,work_order_id,order_item_id,quantity) VALUES(1,$1,$2,1)`, workID, f.items[0]); e != nil {
		t.Fatal(e)
	}
	worker := auth.Principal{OrgID: 1, SubjectID: 999999, Role: "WORKER"}
	saved, e = f.svc.UpdateDemand(ctx, f.p, f.order, "in-service", f.write(1, "上门前电话联系", fmt.Sprint(f.mediaID)))
	if e != nil {
		t.Fatal(e)
	}
	detail, e := fulfillment.New(f.db, f.ms).WorkerWorkOrder(ctx, worker, workID)
	if e != nil || len(detail.Items) != 1 || detail.Items[0].CustomerNote != "上门前电话联系" || len(detail.Items[0].CustomerMedia) != 1 {
		t.Fatalf("worker stale %+v %v", detail, e)
	}
	admin, e := f.svc.Detail(ctx, f.order, 1)
	if e != nil || admin.Items[0].DemandDescription != "上门前电话联系" {
		t.Fatalf("admin stale %+v %v", admin, e)
	}
	_, file, e := f.ms.Read(ctx, f.mediaID, &worker, false)
	if e != nil {
		t.Fatal(e)
	}
	file.Close()
	stranger := worker
	stranger.SubjectID++
	if _, file, e = f.ms.Read(ctx, f.mediaID, &stranger, false); e == nil {
		file.Close()
		t.Fatal("unassigned worker read media")
	}
	saved, e = f.svc.UpdateDemand(ctx, f.p, f.order, "remove-media", f.write(2, "更新后无需附件"))
	if e != nil || len(saved.Items[0].DemandMedia) != 0 {
		t.Fatal(e)
	}
	if e = f.ms.Delete(ctx, f.mediaID, f.p); e == nil {
		t.Fatal("historical evidence physically deleted")
	}
	var count, version int
	var total int64
	var snapshot string
	f.db.QueryRow(`SELECT count(*) FROM order_demand_history WHERE order_id=$1`, f.order).Scan(&count)
	f.db.QueryRow(`SELECT version,total_amount FROM customer_order WHERE id=$1`, f.order).Scan(&version, &total)
	f.db.QueryRow(`SELECT service_description_snapshot FROM order_item WHERE id=$1`, f.items[0]).Scan(&snapshot)
	if count != 3 || version != 0 || total != 23600 || snapshot != "不可修改的成交说明" {
		t.Fatal("transaction facts changed", count, version, total, snapshot)
	}
	for _, status := range []string{"WAITING_ACCEPTANCE", "COMPLETED", "CANCELLED", "CLOSED"} {
		f.db.Exec(`UPDATE customer_order SET status=$1 WHERE id=$2`, status, f.order)
		if _, e = f.svc.UpdateDemand(ctx, f.p, f.order, "closed-"+status, f.write(3, "不允许")); e == nil {
			t.Fatal("terminal update", status)
		}
	}
}
func TestOrderDemandConcurrentUpdates(t *testing.T) {
	f := newDemandFixture(t)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := f.svc.UpdateDemand(context.Background(), f.p, f.order, fmt.Sprint("parallel-", i), f.write(0, fmt.Sprint(i)))
			errors <- e
		}(i)
	}
	wg.Wait()
	close(errors)
	success, conflict := 0, 0
	for e := range errors {
		if e == nil {
			success++
		} else {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("lost update", success, conflict)
	}
	got, e := f.svc.GetDemand(context.Background(), f.p, f.order)
	if e != nil || got.DemandVersion != 1 {
		t.Fatal(got, e)
	}
	tx, e := f.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE customer_order SET status='COMPLETED' WHERE id=$1`, f.order); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		_, e := f.svc.UpdateDemand(context.Background(), f.p, f.order, "finishing", f.write(1, "race"))
		done <- e
	}()
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("write after completion accepted")
	}
}

func TestDemandRejectsInvalidItemSetsAndWrongMediaPurpose(t *testing.T) {
	f := newDemandFixture(t)
	ctx := context.Background()
	invalid := []DemandWrite{f.write(-1, "版本错误"), {DemandVersion: 0, Items: []DemandInput{{ItemID: f.items[0]}}}, {DemandVersion: 0, Items: []DemandInput{{ItemID: f.items[0]}, {ItemID: f.items[0]}}}, f.write(0, strings.Repeat("字", 501)), f.write(0, "重复资料", fmt.Sprint(f.mediaID), fmt.Sprint(f.mediaID))}
	for i, w := range invalid {
		if _, e := f.svc.UpdateDemand(ctx, f.p, f.order, fmt.Sprint("invalid-", i), w); e == nil {
			t.Fatal("invalid update accepted", i)
		}
	}
	for _, field := range []string{"purpose='AVATAR'", "status='VOID'"} {
		if _, e := f.db.Exec(`UPDATE media_asset SET `+field+` WHERE id=$1`, f.mediaID); e != nil {
			t.Fatal(e)
		}
		if _, e := f.svc.UpdateDemand(ctx, f.p, f.order, field, f.write(0, "错误媒体", fmt.Sprint(f.mediaID))); e == nil {
			t.Fatal("bad media accepted", field)
		}
		f.db.Exec(`UPDATE media_asset SET purpose='DEMAND_EVIDENCE',status='READY' WHERE id=$1`, f.mediaID)
	}
	got, e := f.svc.GetDemand(ctx, f.p, f.order)
	if e != nil || got.DemandVersion != 0 || got.Items[0].DemandDescription != "" {
		t.Fatal("invalid writes changed order", got, e)
	}
}

func TestOrderDemandHTTPContract(t *testing.T) {
	f := newDemandFixture(t)
	h := NewHandler(f.svc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/mini/orders/{id}/demand", h.Demand)
	mux.HandleFunc("PUT /api/v1/mini/orders/{id}/demand", h.Demand)
	call := func(method string, w DemandWrite, key string, p auth.Principal) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(w)
		r := httptest.NewRequest(method, fmt.Sprintf("/api/v1/mini/orders/%d/demand", f.order), strings.NewReader(string(raw)))
		r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		r.Header.Set("Idempotency-Key", key)
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, r)
		return out
	}
	out := call("GET", DemandWrite{}, "", f.p)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"demandVersion":0`) {
		t.Fatal(out.Code, out.Body.String())
	}
	out = call("PUT", f.write(0, "HTTP更新"), "http-save", f.p)
	if out.Code != 200 || !strings.Contains(out.Body.String(), `"demandVersion":1`) {
		t.Fatal(out.Code, out.Body.String())
	}
	out = call("PUT", f.write(0, "过期版本"), "http-stale", f.p)
	if out.Code != 409 || !strings.Contains(out.Body.String(), "ORDER_DEMAND_VERSION_CONFLICT") {
		t.Fatal(out.Code, out.Body.String())
	}
	other := f.p
	other.SubjectID++
	out = call("GET", DemandWrite{}, "", other)
	if out.Code != 404 {
		t.Fatal(out.Code, out.Body.String())
	}
	out = call("PUT", f.write(1, "缺幂等标识"), "", f.p)
	if out.Code != 400 {
		t.Fatal(out.Code, out.Body.String())
	}
}
