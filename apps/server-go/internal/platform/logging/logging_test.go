package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSafetySamplingAndReservedFields(t *testing.T) {
	var b bytes.Buffer
	l, e := New(Defaults(), &b)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 1000; i++ {
		l.Info("same event", zap.String("event", "test"), zap.String("service", "forged"), zap.Any("payload", map[string]string{"password": "secret-sentinel"}), zap.String("Authorization", "secret-sentinel"))
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 1000 {
		t.Fatalf("sampling dropped events: %d", len(lines))
	}
	if strings.Contains(b.String(), "secret-sentinel") {
		t.Fatal("sensitive field escaped boundary")
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &m)
	if m["service"] != "fix-pro-api" || m["version"] == nil || m["caller"] == nil {
		t.Fatalf("metadata: %v", m)
	}
}
func TestSafeRootCause(t *testing.T) {
	var b bytes.Buffer
	l, _ := New(Defaults(), &b)
	ctx := Context(context.Background(), l, Defaults(), "req-1")
	err := &pgconn.PgError{Code: "23505", Detail: "phone=secret-sentinel", Message: "secret-sentinel", ConstraintName: "secret-sentinel"}
	QueryFailure(ctx, "customer.Login", "sql_fixed")
	Root(ctx, err, "INTERNAL_ERROR")
	Root(ctx, err, "INTERNAL_ERROR")
	if strings.Contains(b.String(), "secret-sentinel") {
		t.Fatal("raw pg error leaked")
	}
	if strings.Count(b.String(), "http.request.failed") != 1 {
		t.Fatal("duplicate root cause")
	}
	var m map[string]any
	_ = json.Unmarshal(b.Bytes(), &m)
	if m["sqlState"] != "23505" || m["requestId"] != "req-1" || m["operation"] != "customer.Login" || m["stack"] == nil {
		t.Fatalf("missing diagnostics: %v", m)
	}
}
func TestLimitsAndConfig(t *testing.T) {
	c := Defaults()
	c.MaxBytes = 4096
	var b bytes.Buffer
	l, _ := New(c, &b)
	l.Error("bounded", zap.String("requestId", "req-2"), zap.String("errorMessage", strings.Repeat("\x00💡", 10000)), zap.String("stack", strings.Repeat("frame", 10000)))
	if b.Len() > c.MaxBytes {
		t.Fatalf("event %d exceeds %d", b.Len(), c.MaxBytes)
	}
	var m map[string]any
	if json.Unmarshal(b.Bytes(), &m) != nil || m["requestId"] != "req-2" || m["truncated"] != true {
		t.Fatal("invalid truncation")
	}
	for _, level := range []string{"fatal", "bad", "INFO"} {
		c := Defaults()
		c.Level = level
		if c.Validate() == nil {
			t.Fatal("invalid level accepted")
		}
	}
	c = Defaults()
	c.Env = "production"
	if c.Validate() == nil {
		t.Fatal("production missing version accepted")
	}
	c.Version = "1.0"
	c.GitCommit = "abcdef"
	c.InstanceID = "server-1"
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	t.Setenv("DB_SLOW_THRESHOLD", "-1s")
	if _, e := Load("local"); e == nil {
		t.Fatal("negative duration accepted")
	}
}
func TestJobsRecoverContinueAndCount(t *testing.T) {
	var b bytes.Buffer
	l, _ := New(Defaults(), &b)
	RunJob(context.Background(), l, Defaults(), "cleanup", func(ctx context.Context) error { panic("secret-sentinel") })
	RunJob(context.Background(), l, Defaults(), "cleanup", func(ctx context.Context) error { Count(ctx, true); return nil })
	if strings.Contains(b.String(), "secret-sentinel") {
		t.Fatal("panic value leaked")
	}
	if !strings.Contains(b.String(), "job.panic") || !strings.Contains(b.String(), "job.completed") || !strings.Contains(b.String(), `"processedCount":1`) {
		t.Fatalf("missing recovery/count: %s", b.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	RunJob(ctx, l, Defaults(), "cleanup", func(context.Context) error { return nil })
	if !strings.Contains(b.String(), "client_cancelled") {
		t.Fatal("cancelled job marked success")
	}
}
func TestUnknownErrorsAreNotSerialized(t *testing.T) {
	for _, err := range []error{errors.New("secret-sentinel"), context.DeadlineExceeded} {
		var b bytes.Buffer
		l, _ := New(Defaults(), &b)
		l.Error("failure", SafeError(err)...)
		if strings.Contains(b.String(), "secret-sentinel") {
			t.Fatal("error text leaked")
		}
	}
}
func BenchmarkZapDiagnosticEvent(b *testing.B) {
	l, _ := New(Defaults(), io.Discard)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Info("request completed", zap.String("event", "http.request.completed"), zap.String("requestId", "bench-request"), zap.Int("status", 200), zap.Int64("durationMs", time.Since(time.Now()).Milliseconds()))
	}
}
