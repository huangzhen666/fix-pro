package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/fixpro/server/internal/platform/logging"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"runtime"
	"strings"
	"time"
)

type queryKey struct{}
type queryStart struct {
	started         time.Time
	name, operation string
}
type tracer struct{}

func (tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	sum := sha256.Sum256([]byte(d.SQL))
	name := "sql_" + hex.EncodeToString(sum[:6])
	op := "database.query"
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if strings.Contains(f.Function, "github.com/fixpro/server/internal/") && !strings.Contains(f.Function, "/database.") {
			op = f.Function
			break
		}
		if !more {
			break
		}
	}
	return context.WithValue(ctx, queryKey{}, queryStart{time.Now(), name, op})
}
func (tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	q, ok := ctx.Value(queryKey{}).(queryStart)
	if !ok {
		return
	}
	duration := time.Since(q.started)
	threshold := 200 * time.Millisecond
	if s := logging.StateFrom(ctx); s != nil {
		threshold = s.Config.DBSlow
	}
	f := []zap.Field{zap.String("module", "database"), zap.String("queryName", q.name), zap.String("operation", q.operation), zap.Int64("durationMs", duration.Milliseconds())}
	if d.Err != nil {
		logging.QueryFailure(ctx, q.operation, q.name)
		return
	} // HTTP/Job boundary owns root cause.
	if duration >= threshold {
		logging.FromContext(ctx).Warn("slow database operation", append(f, zap.String("event", "db.query.slow"))...)
	}
}
