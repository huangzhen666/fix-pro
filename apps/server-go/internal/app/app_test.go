package app

import (
	"database/sql"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fixpro/server/internal/platform/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPublicPingAndAdminGuard(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://invalid:invalid@127.0.0.1:1/invalid?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := config.Config{Env: "local", MediaDriver: "local", MediaLocalRoot: t.TempDir(), AdminUsername: "admin", AdminPassword: "secret"}
	h, err := New(c, db, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}

	ping := httptest.NewRecorder()
	h.ServeHTTP(ping, httptest.NewRequest(http.MethodGet, "/api/v1/public/ping", nil))
	if ping.Code != http.StatusOK || !strings.Contains(ping.Body.String(), `"code":"OK"`) {
		t.Fatalf("ping: %d %s", ping.Code, ping.Body.String())
	}

	admin := httptest.NewRecorder()
	h.ServeHTTP(admin, httptest.NewRequest(http.MethodGet, "/api/v1/admin/catalog/categories", nil))
	if admin.Code != http.StatusUnauthorized {
		t.Fatalf("admin status=%d", admin.Code)
	}
}

func TestAllCustomerRoutesRejectLegacyIdentity(t *testing.T) {
	db, _ := sql.Open("pgx", "postgres://invalid:invalid@127.0.0.1:1/invalid?sslmode=disable")
	defer db.Close()
	h, e := New(config.Config{Env: "local", MediaDriver: "local", MediaLocalRoot: t.TempDir()}, db, zap.NewNop())
	if e != nil {
		t.Fatal(e)
	}
	for _, route := range []string{"POST auth/session/verify", "GET auth/me", "GET profile", "PUT profile", "GET checkout/contact", "POST media/avatar", "POST media/fault", "GET media/1/content", "DELETE media/1", "GET cart", "POST cart/items", "PATCH cart/items/1", "PUT cart/items/1/fault", "DELETE cart/items/1", "POST orders", "POST orders/1/repeat", "GET addresses", "POST addresses", "PUT addresses/1", "POST addresses/1/default", "DELETE addresses/1", "GET orders", "GET orders/1", "GET work-orders/1", "POST work-orders/1/acceptance", "POST work-orders/1/rating", "GET work-orders/1/timeline"} {
		method, path, _ := strings.Cut(route, " ")
		r := httptest.NewRequest(method, "/api/v1/mini/"+path, nil)
		r.Header.Set("Authorization", "Bearer local-customer-1")
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, r)
		if rw.Code != 401 {
			t.Fatalf("%s: %d", route, rw.Code)
		}
	}
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, httptest.NewRequest("POST", "/api/v1/mini/auth/logout", nil))
	if rw.Code != 404 {
		t.Fatalf("customer logout registered: %d", rw.Code)
	}
}
