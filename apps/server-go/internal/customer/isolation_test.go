package customer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/fixpro/server/internal/address"
	"github.com/fixpro/server/internal/authorization"
	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/media"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type fixedPrincipal struct{ p auth.Principal }

func (f fixedPrincipal) Authenticate(*http.Request) (auth.Principal, error) { return f.p, nil }

func TestIntegrationPrivateResourcesAndUsage(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DSN")
	if dsn == "" {
		t.Skip("disposable CUSTOMER_TEST_DSN required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	var name string
	db.QueryRow(`SELECT current_database()`).Scan(&name)
	if len(name) < 5 || name[len(name)-5:] != "_test" {
		t.Fatal("disposable DB required")
	}
	s := New(db, fakeGateway{}, codecForTest(t), 1, "test-app", time.Hour)
	stamp := time.Now().UnixNano()
	root := t.TempDir()
	ms, e := media.New(db, root)
	if e != nil {
		t.Fatal(e)
	}
	_, e = Initialize(ctx, db, root, false, false)
	if e != nil {
		t.Fatal(e)
	}
	login := func(n int64) (int64, string) {
		out, e := s.Login(ctx, fmt.Sprintf("137%08d", n%100000000), fmt.Sprint(n))
		if e != nil {
			t.Fatal(e)
		}
		m := out.(map[string]any)
		id, _ := strconv.ParseInt(m["customer"].(Profile).ID, 10, 64)
		return id, m["accessToken"].(string)
	}
	a, tokenA := login(stamp)
	b, _ := login(stamp + 1)
	pa := auth.Principal{OrgID: 1, SubjectID: a, Role: "CUSTOMER"}
	pb := auth.Principal{OrgID: 1, SubjectID: b, Role: "CUSTOMER"}
	defer func() {
		for _, table := range []string{"customer_session", "customer_wechat_identity", "customer_address"} {
			db.Exec("DELETE FROM "+table+" WHERE customer_id IN($1,$2)", a, b)
		}
		db.Exec(`DELETE FROM customer WHERE id IN($1,$2)`, a, b)
	}()
	expect404 := func(e error) {
		t.Helper()
		var x *httpx.Error
		if !errors.As(e, &x) || x.Status != 404 {
			t.Fatalf("expected 404, got %v", e)
		}
	}
	addr := address.New(db)
	ad, e := addr.Create(ctx, pb, address.Write{City: "汉川市", DetailAddress: "测试服务地址123号", BuildingDoor: "101", ContactName: "测试客户", ContactMobile: "13800138000"})
	if e != nil {
		t.Fatal(e)
	}
	aid, _ := strconv.ParseInt(ad.ID, 10, 64)
	expect404(addr.Delete(ctx, pa, aid))
	var cartID, itemID int64
	db.QueryRow(`INSERT INTO shopping_cart(org_id,customer_id) VALUES(1,$1) RETURNING id`, b).Scan(&cartID)
	db.QueryRow(`INSERT INTO shopping_cart_item(org_id,cart_id,sku_id,sku_version,quantity,unit_price) VALUES(1,$1,1001,1,1,100) RETURNING id`, cartID).Scan(&itemID)
	defer db.Exec(`DELETE FROM shopping_cart WHERE id=$1`, cartID)
	defer db.Exec(`DELETE FROM shopping_cart_item WHERE id=$1`, itemID)
	_, e = cart.New(db).Quantity(ctx, pa, itemID, 2)
	expect404(e)
	var orderID, workID, submission int64
	e = db.QueryRow(`INSERT INTO customer_order(org_id,order_no,customer_id,contact_name,contact_mobile,service_address,order_type,status,total_amount,paid_amount,item_count) VALUES(1,$1,$2,'测试','13800138000','测试地址','REPAIR','COMPLETED',500,300,2) RETURNING id`, fmt.Sprintf("ISOLATION-%d", stamp), b).Scan(&orderID)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec(`DELETE FROM customer_order WHERE id=$1`, orderID)
	e = db.QueryRow(`INSERT INTO work_order(org_id,work_order_no,order_id,status,priority,customer_acceptance_status) VALUES(1,$1,$2,'FINISHED','NORMAL','MANUAL_ACCEPTED') RETURNING id`, fmt.Sprintf("ISOLATION-WO-%d", stamp), orderID).Scan(&workID)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec(`DELETE FROM work_order WHERE id=$1`, workID)
	db.QueryRow(`INSERT INTO completion_submission(org_id,work_order_id,attempt_no,worker_id,summary) VALUES(1,$1,1,1,'完成') RETURNING id`, workID).Scan(&submission)
	defer db.Exec(`DELETE FROM completion_submission WHERE id=$1`, submission)
	defer db.Exec(`DELETE FROM customer_rating WHERE work_order_id=$1`, workID)
	fs := fulfillment.New(db, ms)
	_, e = fs.CustomerOrder(ctx, pa, orderID)
	expect404(e)
	_, e = fs.CustomerWorkOrder(ctx, pa, workID)
	expect404(e)
	_, e = fs.WorkOrderTimeline(ctx, pa, workID)
	expect404(e)
	expect404(fs.SubmitRating(ctx, pa, workID, fulfillment.RatingRequest{Stars: 5}))
	if e = fs.SubmitRating(ctx, pb, workID, fulfillment.RatingRequest{Stars: 5}); e != nil {
		t.Fatal(e)
	}
	if e = fs.SubmitRating(ctx, pb, workID, fulfillment.RatingRequest{Stars: 5}); e == nil {
		t.Fatal("duplicate rating accepted")
	}
	var mid int64
	key := fmt.Sprintf("customer_avatar/test-%d.png", stamp)
	os.MkdirAll(filepath.Dir(filepath.Join(root, key)), 0750)
	os.WriteFile(filepath.Join(root, key), []byte("fixture"), 0600)
	e = db.QueryRow(`INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES(1,'CUSTOMER',$1,'CUSTOMER_AVATAR','IMAGE','avatar.png',$2,'image/png',7,'READY') RETURNING id`, b, key).Scan(&mid)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec(`DELETE FROM media_asset WHERE id=$1`, mid)
	_, f, e := ms.Read(ctx, mid, &pa, false)
	if f != nil {
		f.Close()
	}
	expect404(e)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	session, _ := s.Session(req)
	p, _ := s.Profile(ctx, 1, a)
	mediaID := fmt.Sprint(mid)
	_, e = s.UpdateProfile(ctx, session, ProfileWrite{DisplayName: p.DisplayName, AvatarMediaID: &mediaID, Version: &p.Version})
	expect404(e)
	db.Exec(`UPDATE customer SET avatar_media_id=$1 WHERE id=$2`, mid, b)
	defer db.Exec(`UPDATE customer SET avatar_media_id=NULL WHERE id=$1`, b)
	if e = ms.Delete(ctx, mid, pb); e == nil {
		t.Fatal("referenced avatar deleted")
	}
	db.Exec(`UPDATE media_asset SET created_at=now()-interval '25 hours' WHERE id=$1`, mid)
	if e = ms.CollectAvatars(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(root, key)); e != nil {
		t.Fatal("referenced avatar collected")
	}
	db.Exec(`UPDATE customer SET avatar_media_id=NULL WHERE id=$1`, b)
	if e = ms.CollectAvatars(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(root, key)); !os.IsNotExist(e) {
		t.Fatal("orphan avatar not collected")
	}
	handler := NewHandler(s)
	rw := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("id", fmt.Sprint(b))
	r.SetBasicAuth("test", "test")
	auth.Admin("test", "test", http.HandlerFunc(handler.AdminDetail)).ServeHTTP(rw, r)
	if rw.Code != 200 {
		t.Fatal(rw.Body.String())
	}
	var out struct {
		Data struct{ Customer AdminCustomer }
	}
	json.Unmarshal(rw.Body.Bytes(), &out)
	if out.Data.Customer.Usage.OrderCount != 1 || out.Data.Customer.Usage.PaidAmount != 300 {
		t.Fatal("usage amount/count incorrect", out.Data.Customer.Usage)
	}
	db.Exec(`UPDATE customer SET status='DISABLED' WHERE id=$1`, a)
	if _, e = s.Session(req); e == nil {
		t.Fatal("disabled session accepted")
	}
	db.Exec(`UPDATE customer SET status='ACTIVE',deleted_at=now() WHERE id=$1`, a)
	if _, e = s.Session(req); e == nil {
		t.Fatal("deleted session accepted")
	}
	var org int64
	e = db.QueryRow(`INSERT INTO organization(name,status) VALUES($1,'ACTIVE') RETURNING id`, fmt.Sprint(stamp)).Scan(&org)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		db.Exec(`DELETE FROM admin_role_permission WHERE org_id=$1`, org)
		db.Exec(`DELETE FROM admin_role WHERE org_id=$1`, org)
		db.Exec(`DELETE FROM organization WHERE id=$1`, org)
	}()
	var permissionExists bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM admin_role_permission rp JOIN admin_role r ON r.id=rp.role_id JOIN admin_permission p ON p.id=rp.permission_id WHERE r.org_id=$1 AND r.role_code='tenant_admin' AND p.permission_code='customer.view')`, org).Scan(&permissionExists)
	if !permissionExists {
		t.Fatal("new org missing customer permission")
	}
	for _, fn := range []http.HandlerFunc{handler.AdminDetail, handler.AdminOrders} {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue("id", fmt.Sprint(b))
		auth.Customer(fixedPrincipal{auth.Principal{OrgID: org, Role: "ADMIN"}}, fn).ServeHTTP(rw, r)
		if rw.Code != 404 {
			t.Fatal("cross org customer visible", rw.Code)
		}
	}
	authorizer := authorization.New(db)
	allowed, e := authorizer.Check(ctx, authorization.Subject{UserID: 999999999, OrgID: org}, "customer.view")
	if e != nil || allowed {
		t.Fatal("unassigned user granted permission", e)
	}
}
