package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sort"
	"strings"
	"unicode/utf8"
)

// Closed field vocabulary prevents accidentally dumping a DTO or credential.
var allowed = map[string]bool{}

func init() {
	for _, k := range strings.Fields("module event requestId principalType principalId orgId method route path status durationMs errorCode orderId workOrderId skuId mediaId errorType errorMessage operation caller stack dependency upstreamCode queryName sqlState jobName jobRunId result processedCount failedCount responseCommitted truncated fromStatus toStatus bytes contentType retryCount open inUse idle waitCount waitDurationMs waitCountDelta waitDurationMsDelta size clientGenerated") {
		allowed[k] = true
	}
}

type safeCore struct {
	zapcore.Core
	max   int
	bound []zap.Field
}

func (c *safeCore) With(f []zap.Field) zapcore.Core {
	out := *c
	out.bound = append(append([]zap.Field{}, c.bound...), f...)
	return &out
}
func (c *safeCore) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(e.Level) {
		return ce.AddCore(e, c)
	}
	return ce
}
func (c *safeCore) Write(e zapcore.Entry, fields []zap.Field) error {
	// Budget leaves room for encoder metadata, caller and resource attributes.
	budget := c.max - 1536
	seen := map[string]bool{}
	out := []zap.Field{}
	truncated := false
	all := append(append([]zap.Field{}, c.bound...), fields...)
	// Retain identifiers before any potentially large stack/message fields.
	priority := func(k string) int {
		switch k {
		case "requestId", "jobRunId", "orgId", "principalType", "principalId", "orderId", "workOrderId", "mediaId", "event", "module", "errorCode", "status":
			return 0
		case "stack", "errorMessage":
			return 2
		default:
			return 1
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return priority(all[i].Key) < priority(all[j].Key) })
	for _, f := range all {
		if !allowed[f.Key] || seen[f.Key] {
			continue
		}
		seen[f.Key] = true
		switch f.Type {
		case zapcore.StringType:
			cap := 2048
			if f.Key == "requestId" || f.Key == "jobRunId" {
				cap = 64
			}
			if len(f.String) > cap {
				f.String = cut(f.String, cap)
				truncated = true
			}
		case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type, zapcore.Uint64Type, zapcore.Uint32Type, zapcore.Uint16Type, zapcore.Uint8Type, zapcore.BoolType, zapcore.Float64Type, zapcore.Float32Type:
		default:
			continue
		}
		cost := len(f.Key) + len(f.String)*6 + 32 // worst-case JSON escaping
		if cost > budget {
			if f.Type == zapcore.StringType && budget > 128 {
				f.String = cut(f.String, (budget-len(f.Key)-32)/6)
				cost = len(f.Key) + len(f.String)*6 + 32
				truncated = true
			} else {
				truncated = true
				continue
			}
		}
		budget -= cost
		out = append(out, f)
	}
	if truncated {
		out = append(out, zap.Bool("truncated", true))
	}
	e.Message = cut(e.Message, 128)
	e.Stack = "" // stack only through the safe frame formatter
	// Do not expose host source directory in caller.
	file := e.Caller.File
	if i := strings.Index(file, "/internal/"); i >= 0 {
		e.Caller.File = file[i+1:]
	} else if i := strings.LastIndex(file, "/"); i >= 0 {
		e.Caller.File = file[i+1:]
	}
	return c.Core.Write(e, out)
}
func cut(s string, n int) string {
	if n < 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}
