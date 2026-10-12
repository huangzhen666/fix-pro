package database

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHistoryCorrelationAndSafeQueryFailure(t *testing.T) {
	if os.Getenv("FIXPRO_INTEGRATION") != "1" {
		t.Skip("requires isolated PostgreSQL")
	}
	dsn := os.Getenv("DB_DSN")
	if !strings.Contains(dsn, "fixpro_logging_test") {
		t.Skip("isolated fault database required")
	}
	c := config.Config{DBDSN: dsn, DBMaxOpen: 1, DBMaxIdle: 1}
	db, e := Open(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var output bytes.Buffer
	lc := logging.Defaults()
	lc.DBSlow = time.Millisecond
	l, _ := logging.New(lc, &output)
	ctx := logging.Context(context.Background(), l, lc, "history-req")
	logging.Bind(ctx, zap.String("jobRunId", "history-job"))
	tx, e := BeginTx(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	var req, job string
	e = tx.QueryRowContext(ctx, `INSERT INTO order_status_history(org_id,order_id,to_status,event_code,operator_type) VALUES(1,0,'TEST','LOGGING_TEST','SYSTEM') RETURNING request_id,job_run_id`).Scan(&req, &job)
	if e != nil {
		t.Fatal(e)
	}
	if req != "history-req" || job != "history-job" {
		t.Fatal("missing history IDs")
	}
	_ = tx.Rollback()
	var stale string
	if e = db.QueryRow(`SELECT COALESCE(current_setting('fixpro.request_id',true),'')`).Scan(&stale); e != nil || stale != "" {
		t.Fatalf("pooled connection leaked ID %q %v", stale, e)
	}
	_, e = db.ExecContext(ctx, `SELECT 'secret-sentinel'::integer`)
	if e == nil {
		t.Fatal("expected invalid input")
	}
	logging.Root(ctx, e, "INTERNAL_ERROR")
	if strings.Contains(output.String(), "secret-sentinel") {
		t.Fatal("SQL leaked")
	}
	if !strings.Contains(output.String(), "sqlState") || !strings.Contains(output.String(), "queryName") {
		t.Fatal("missing query diagnostic")
	}
	_, e = db.ExecContext(ctx, `SELECT pg_sleep(0.01)`)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, line := range strings.Split(output.String(), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(line), &m)
		if m["event"] == "db.query.slow" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing slow query event")
	}
}
