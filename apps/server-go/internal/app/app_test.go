package app

import (
	"database/sql"
	"io"
	"log/slog"
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
	h, err := New(c, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
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
