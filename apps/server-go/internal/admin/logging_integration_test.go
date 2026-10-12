package admin

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/authorization"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/database"
	"github.com/fixpro/server/internal/platform/logging"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResetPasswordAuditAtomicity(t *testing.T) {
	if os.Getenv("FIXPRO_INTEGRATION") != "1" {
		t.Skip("requires isolated PostgreSQL")
	}
	dsn := os.Getenv("DB_DSN")
	if !strings.Contains(dsn, "fixpro_logging_test") {
		t.Skip("fault injection only runs in fixpro_logging_test")
	}
	c := config.Config{DBDSN: dsn, DBMaxOpen: 5, DBMaxIdle: 2}
	db, e := database.Open(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var id int64
	name := fmt.Sprintf("logtest-%d", time.Now().UnixNano())
	e = db.QueryRow(`INSERT INTO admin_user(org_id,username,display_name,password_hash) VALUES(1,$1,'logging test','original-hash') RETURNING id`, name).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec(`DELETE FROM admin_user WHERE id=$1`, id)
	defer db.Exec(`DELETE FROM audit_log WHERE resource_id=$1`, strconv.FormatInt(id, 10))
	// Dedicated resource condition keeps failure injection away from other tests.
	fn := "logging_audit_test_" + strconv.FormatInt(id, 10)
	_, e = db.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.resource_id='%d' THEN RAISE EXCEPTION 'secret-sentinel'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION %s()`, fn, id, fn, fn))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DROP TRIGGER IF EXISTS " + fn + " ON audit_log; DROP FUNCTION IF EXISTS " + fn + "()")
	var output bytes.Buffer
	l, _ := logging.New(logging.Defaults(), &output)
	handler := NewHandler(db, auth.NewSessionStore(db, false), authorization.New(db))
	run := func(reqID string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/reset", nil)
		r.SetPathValue("id", strconv.FormatInt(id, 10))
		ctx := logging.Context(r.Context(), l, logging.Defaults(), reqID)
		ctx = auth.WithPrincipal(ctx, auth.Principal{OrgID: 1, AdminUserID: id, SubjectID: id, Role: "ADMIN"})
		w := httptest.NewRecorder()
		handler.ResetPassword(w, r.WithContext(ctx))
		return w
	}
	w := run("audit-failure")
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var hash string
	if e = db.QueryRow(`SELECT password_hash FROM admin_user WHERE id=$1`, id).Scan(&hash); e != nil {
		t.Fatal(e)
	}
	if hash != "original-hash" {
		t.Fatal("password committed without audit")
	}
	if strings.Contains(output.String(), "secret-sentinel") {
		t.Fatal("pg error leaked")
	}
	if _, e = db.Exec("DROP TRIGGER " + fn + " ON audit_log"); e != nil {
		t.Fatal(e)
	}
	w = run("audit-success")
	if w.Code != 200 {
		t.Fatalf("reset: %s", w.Body.String())
	}
	var req sql.NullString
	e = db.QueryRow(`SELECT request_id FROM audit_log WHERE resource_id=$1 AND action_code='admin_user.reset_password' ORDER BY id DESC LIMIT 1`, strconv.FormatInt(id, 10)).Scan(&req)
	if e != nil || req.String != "audit-success" {
		t.Fatalf("audit correlation %v %v", req, e)
	}
}
