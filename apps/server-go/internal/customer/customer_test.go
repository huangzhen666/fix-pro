package customer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/wechat"
	_ "github.com/jackc/pgx/v5/stdlib"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func codecForTest(t *testing.T) *MobileCodec {
	t.Helper()
	c, e := NewMobileCodec(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestMobileAndNickname(t *testing.T) {
	c := codecForTest(t)
	n, e := NormalizePhone("13800138000")
	if e != nil || n != "+8613800138000" {
		t.Fatal(n, e)
	}
	a, _ := c.Encrypt(n)
	b, _ := c.Encrypt(n)
	if bytes.Equal(a, b) {
		t.Fatal("nonce reused")
	}
	v, e := c.Decrypt(a)
	if e != nil || v != n {
		t.Fatal(e)
	}
	a[len(a)-1] ^= 1
	if _, e = c.Decrypt(a); e == nil {
		t.Fatal("tampering accepted")
	}
	if !bytes.Equal(c.Hash(n), c.Hash(n)) || MaskPhone(n) != "138****8000" {
		t.Fatal("hash/mask")
	}
	for _, s := range []string{"", "138", "+1 1234567890"} {
		if _, e = NormalizePhone(s); e == nil {
			t.Fatal("accepted invalid phone")
		}
	}
	for _, s := range []string{"", strings.Repeat("名", 33), "昵称\n换行"} {
		if _, e = validateName(s); e == nil {
			t.Fatal("accepted invalid name")
		}
	}
}

type fakeGateway struct{}

func (fakeGateway) Identity(_ context.Context, code string) (wechat.Identity, error) {
	return wechat.Identity{OpenID: code}, nil
}
func (fakeGateway) Phone(_ context.Context, code string) (wechat.Phone, error) {
	p := wechat.Phone{Number: code, Country: "86"}
	p.Watermark.AppID = "test-app"
	return p, nil
}
func (fakeGateway) CheckNickname(context.Context, string, string) error { return nil }
func TestIntegrationInitializeAndAccounts(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DSN")
	if dsn == "" {
		t.Skip("CUSTOMER_TEST_DSN required: disposable database only")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	ctx := context.Background()
	var name string
	db.QueryRow(`SELECT current_database()`).Scan(&name)
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("disposable test database required")
	}
	var catalog, workers, admins int
	db.QueryRow(`SELECT count(*) FROM service_sku`).Scan(&catalog)
	db.QueryRow(`SELECT count(*) FROM employee_account`).Scan(&workers)
	db.QueryRow(`SELECT count(*) FROM admin_user`).Scan(&admins)
	root := t.TempDir()
	report, e := Initialize(ctx, db, root, false, false)
	if e != nil {
		t.Fatal(e)
	}
	if !report.Completed {
		report, e = Initialize(ctx, db, root, true, false)
		if e != nil {
			t.Fatal(e)
		}
	}
	if !report.Completed {
		t.Fatal("initialization marker missing")
	}
	if _, e = Initialize(ctx, db, root, true, false); e == nil {
		t.Fatal("repeated destructive initialization accepted")
	}
	if _, e = Initialize(ctx, db, root, false, true); e != nil {
		t.Fatal(e)
	}
	for table, want := range map[string]int{"service_sku": catalog, "employee_account": workers, "admin_user": admins} {
		var n int
		db.QueryRow("SELECT count(*) FROM " + table).Scan(&n)
		if n != want {
			t.Fatalf("retained %s changed", table)
		}
	}
	s := New(db, fakeGateway{}, codecForTest(t), 1, "test-app", time.Hour)
	login := func(phone, openid string) (string, string) {
		out, e := s.Login(ctx, phone, openid)
		if e != nil {
			t.Fatal(e)
		}
		m := out.(map[string]any)
		return m["accessToken"].(string), m["customer"].(Profile).ID
	}
	phoneA := "139" + time.Now().Format("150405") + "00"
	phoneB := phoneA[:10] + "1"
	token, id := login(phoneA, "environment-A")
	_, same := login(phoneA, "environment-B")
	if same != id {
		t.Fatal("same phone created multiple accounts")
	}
	otherToken, other := login(phoneB, "environment-A")
	if id == other {
		t.Fatal("different phones shared customer")
	}
	defer db.Exec(`DELETE FROM customer_session WHERE customer_id IN($1,$2)`, id, other)
	defer func() {
		db.Exec(`DELETE FROM customer_wechat_identity WHERE customer_id IN($1,$2)`, id, other)
		db.Exec(`DELETE FROM customer_session WHERE customer_id IN($1,$2)`, id, other)
		db.Exec(`DELETE FROM customer WHERE id IN($1,$2)`, id, other)
	}()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := s.Login(ctx, phoneA, "parallel")
			if e == nil && out.(map[string]any)["customer"].(Profile).ID != id {
				e = sql.ErrNoRows
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	session, e := s.Session(req)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Profile(ctx, 1, session.Principal.SubjectID)
	if e != nil {
		t.Fatal(e)
	}
	version := p.Version
	updated, e := s.UpdateProfile(ctx, session, ProfileWrite{DisplayName: "新昵称", Version: &version})
	if e != nil || updated.DisplayName != "新昵称" {
		t.Fatal(e)
	}
	if _, e = s.UpdateProfile(ctx, session, ProfileWrite{DisplayName: "覆盖", Version: &version}); e == nil {
		t.Fatal("stale version accepted")
	}
	foreign := "999999999"
	version = updated.Version
	if _, e = s.UpdateProfile(ctx, session, ProfileWrite{DisplayName: "新昵称", AvatarMediaID: &foreign, Version: &version}); e == nil {
		t.Fatal("foreign media accepted")
	}
	handler := NewHandler(s)
	verify := httptest.NewRequest("POST", "/", strings.NewReader(`{"loginCode":"environment-B"}`))
	verify.Header.Set("Authorization", "Bearer "+token)
	rw := httptest.NewRecorder()
	handler.Verify(rw, verify)
	if rw.Code != 401 {
		t.Fatal("environment mismatch accepted", rw.Code)
	}
	admin := func(fn http.HandlerFunc, path string) *httptest.ResponseRecorder {
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("GET", path, nil)
		auth.Admin("test", "test", fn).ServeHTTP(rw, func() *http.Request { r.SetBasicAuth("test", "test"); return r }())
		return rw
	}
	rw = admin(handler.Customers, "/?mobile="+phoneA+"&sort=orders")
	if rw.Code != 200 {
		t.Fatal("list query", rw.Body.String())
	}
	var body struct {
		Data struct {
			Total int `json:"total"`
		}
	}
	json.Unmarshal(rw.Body.Bytes(), &body)
	if body.Data.Total != 1 {
		t.Fatal("exact phone filter", body.Data.Total)
	}
	rw = admin(handler.Overview, "/")
	if rw.Code != 200 {
		t.Fatal("overview", rw.Body.String())
	}
	req.Header.Set("Authorization", "Bearer local-customer-1")
	if _, e = s.Session(req); e == nil {
		t.Fatal("legacy token accepted")
	}
	db.Exec(`UPDATE customer_session SET revoked_at=now() WHERE customer_id=$1`, id)
	req.Header.Set("Authorization", "Bearer "+token)
	if _, e = s.Session(req); e == nil {
		t.Fatal("revoked token accepted")
	}
	req.Header.Set("Authorization", "Bearer "+otherToken)
	if _, e = s.Session(req); e != nil {
		t.Fatal("other customer affected", e)
	}
	s.OrgID = 2
	if _, e = s.Session(req); e == nil {
		t.Fatal("cross org session accepted")
	}
	s.OrgID = 1
	db.Exec(`UPDATE customer_session SET expires_at=now()-interval '1 second' WHERE customer_id=$1`, other)
	if _, e = s.Session(req); e == nil {
		t.Fatal("expired session accepted")
	}
}
