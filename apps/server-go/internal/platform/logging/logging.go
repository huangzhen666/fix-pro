// Package logging defines the diagnostic boundary: never serialize arbitrary
// errors, HTTP objects, SDK payloads or credentials into production logs.
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Version = "development"
var GitCommit = "unknown"

type Config struct {
	Level, Format, Service, Env, Version, GitCommit, InstanceID string
	HTTPSlow, MediaSlow, DBSlow, UpstreamSlow, JobSlow          time.Duration
	MaxBytes                                                    int
}

func Defaults() Config {
	return Config{Level: "info", Format: "json", Service: "fix-pro-api", Env: "local", Version: Version, GitCommit: GitCommit, InstanceID: "local", HTTPSlow: time.Second, MediaSlow: 5 * time.Second, DBSlow: 200 * time.Millisecond, UpstreamSlow: 2 * time.Second, JobSlow: 30 * time.Second, MaxBytes: 16384}
}
func Load(env string) (Config, error) {
	c := Defaults()
	c.Env = env
	for k, p := range map[string]*string{"LOG_LEVEL": &c.Level, "LOG_FORMAT": &c.Format, "LOG_SERVICE": &c.Service, "APP_VERSION": &c.Version, "GIT_COMMIT": &c.GitCommit, "LOG_INSTANCE_ID": &c.InstanceID} {
		if v := os.Getenv(k); v != "" {
			*p = v
		}
	}
	for k, p := range map[string]*time.Duration{"HTTP_SLOW_THRESHOLD": &c.HTTPSlow, "MEDIA_SLOW_THRESHOLD": &c.MediaSlow, "DB_SLOW_THRESHOLD": &c.DBSlow, "UPSTREAM_SLOW_THRESHOLD": &c.UpstreamSlow, "JOB_SLOW_THRESHOLD": &c.JobSlow} {
		if v := os.Getenv(k); v != "" {
			d, e := time.ParseDuration(v)
			if e != nil || d <= 0 {
				return c, &configurationError{key: k, reason: "invalid positive duration"}
			}
			*p = d
		}
	}
	if v := os.Getenv("LOG_MAX_EVENT_BYTES"); v != "" {
		n, e := strconv.Atoi(v)
		c.MaxBytes = n
		if e != nil {
			return c, &configurationError{key: "LOG_MAX_EVENT_BYTES", reason: "invalid integer"}
		}
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Level != "debug" && c.Level != "info" && c.Level != "warn" && c.Level != "error" {
		return &configurationError{key: "LOG_LEVEL", reason: "invalid LOG_LEVEL"}
	}
	if c.Format != "json" && c.Format != "console" {
		return &configurationError{key: "LOG_FORMAT", reason: "invalid LOG_FORMAT"}
	}
	if c.Format == "console" && c.Env != "local" && c.Env != "test" {
		return &configurationError{key: "LOG_FORMAT", reason: "console logs require local/test"}
	}
	if c.MaxBytes < 4096 || c.MaxBytes > 65536 {
		return &configurationError{key: "LOG_MAX_EVENT_BYTES", reason: "LOG_MAX_EVENT_BYTES must be 4096..65536"}
	}
	if c.HTTPSlow <= 0 || c.MediaSlow <= 0 || c.DBSlow <= 0 || c.UpstreamSlow <= 0 || c.JobSlow <= 0 {
		return &configurationError{key: "thresholds", reason: "log thresholds must be positive"}
	}
	if c.Env == "production" && (c.Version == "" || c.Version == "development" || c.Version == "unknown" || c.GitCommit == "" || c.GitCommit == "unknown" || c.InstanceID == "" || c.InstanceID == "local") {
		return &configurationError{key: "release metadata", reason: "production log version/commit/instance required"}
	}
	for _, s := range []string{c.Service, c.Env, c.Version, c.GitCommit, c.InstanceID} {
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/+:-]{0,63}$`).MatchString(s) {
			return &configurationError{key: "resource metadata", reason: "invalid log resource metadata"}
		}
	}
	return nil
}

// New deliberately does not use NewProduction's sampling preset.
func New(c Config, output io.Writer) (*zap.Logger, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	if output == nil {
		output = os.Stdout
	}
	var level zapcore.Level
	_ = level.UnmarshalText([]byte(c.Level))
	enc := zap.NewProductionEncoderConfig()
	enc.TimeKey = "timestamp"
	enc.MessageKey = "message"
	enc.EncodeTime = func(t time.Time, e zapcore.PrimitiveArrayEncoder) { e.AppendString(t.UTC().Format(time.RFC3339Nano)) }
	var encoder zapcore.Encoder = zapcore.NewJSONEncoder(enc)
	if c.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(enc)
	}
	base := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(output)), level).With([]zap.Field{zap.String("service", c.Service), zap.String("env", c.Env), zap.String("version", c.Version), zap.String("gitCommit", c.GitCommit), zap.String("instanceId", c.InstanceID)})
	return zap.New(&safeCore{Core: base, max: c.MaxBytes}, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)), nil
}
func Bootstrap(service string) *zap.Logger {
	c := Defaults()
	c.Service = service
	l, _ := New(c, nil)
	return l
}
func ID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("request id entropy unavailable")
	}
	return hex.EncodeToString(b[:])
}

// SafeError extracts only categories. Database Detail, SQL text and URL errors
// frequently contain credentials or personal values, so err.Error is never used.
func SafeError(err error) []zap.Field {
	kind := "unknown_error"
	msg := "internal operation failed"
	var ce *configurationError
	var pg *pgconn.PgError
	var ne net.Error
	switch {
	case errors.As(err, &ce):
		return []zap.Field{zap.String("errorType", "configuration_error"), zap.String("errorMessage", ce.reason), zap.String("operation", ce.key)}
	case errors.As(err, &pg):
		return []zap.Field{zap.String("errorType", "database_error"), zap.String("errorMessage", "database operation failed"), zap.String("sqlState", pg.Code)}
	case errors.Is(err, context.Canceled):
		kind = "client_cancelled"
		msg = "operation cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		kind = "deadline_exceeded"
		msg = "deadline exceeded"
	case errors.As(err, &ne):
		kind = "network_error"
		msg = "network operation failed"
		if ne.Timeout() {
			kind = "dependency_timeout"
			msg = "dependency timed out"
		}
	}
	return []zap.Field{zap.String("errorType", kind), zap.String("errorMessage", msg)}
}

// Stack contains only frame names and repository-relative file names, not
// runtime/debug.Stack's goroutine headers, absolute directories or arguments.
func Stack() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	f := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	for {
		v, more := f.Next()
		file := v.File
		if i := strings.Index(file, "/internal/"); i >= 0 {
			file = file[i+1:]
		} else {
			parts := strings.Split(file, "/")
			file = parts[len(parts)-1]
		}
		fmt.Fprintf(&b, "%s %s:%d\n", v.Function, file, v.Line)
		if !more {
			break
		}
	}
	return b.String()
}

type stateKey struct{}
type State struct {
	mu                   sync.Mutex
	Logger               *zap.Logger
	Config               Config
	ID                   string
	Fields               map[string]zap.Field
	Code                 string
	RootLogged           bool
	Processed, Failed    int
	Operation, QueryName string
}

func Context(ctx context.Context, l *zap.Logger, c Config, id string) context.Context {
	return context.WithValue(ctx, stateKey{}, &State{Logger: l, Config: c, ID: id, Fields: map[string]zap.Field{}, Code: "OK"})
}
func StateFrom(ctx context.Context) *State { s, _ := ctx.Value(stateKey{}).(*State); return s }
func FromContext(ctx context.Context) *zap.Logger {
	s := StateFrom(ctx)
	if s == nil {
		return zap.NewNop()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f := make([]zap.Field, 0, len(s.Fields)+1)
	if s.ID != "" {
		f = append(f, zap.String("requestId", s.ID))
	}
	for _, v := range s.Fields {
		f = append(f, v)
	}
	return s.Logger.With(f...)
}
func Bind(ctx context.Context, fields ...zap.Field) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, f := range fields {
			s.Fields[f.Key] = f
		}
	}
}
func SetCode(ctx context.Context, code string) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		s.Code = code
		s.mu.Unlock()
	}
}
func Code(ctx context.Context) string {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.Code
	}
	return "OK"
}
func Root(ctx context.Context, err error, code string) {
	s := StateFrom(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.RootLogged {
		s.mu.Unlock()
		return
	}
	s.RootLogged = true
	op, q := s.Operation, s.QueryName
	s.mu.Unlock()
	f := append(SafeError(err), zap.String("module", "http"), zap.String("event", "http.request.failed"), zap.String("errorCode", code), zap.String("operation", op), zap.String("queryName", q), zap.String("stack", Stack()))
	FromContext(ctx).WithOptions(zap.AddCallerSkip(1)).Error("request failed", f...)
}
func QueryFailure(ctx context.Context, op, q string) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		s.Operation = op
		s.QueryName = q
		s.mu.Unlock()
	}
}
func Count(ctx context.Context, ok bool) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		if ok {
			s.Processed++
		} else {
			s.Failed++
		}
		s.mu.Unlock()
	}
}
func Counts(ctx context.Context) (int, int) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.Processed, s.Failed
	}
	return 0, 0
}
func Event(ctx context.Context, module, event string, fields ...zap.Field) {
	f := append([]zap.Field{zap.String("module", module), zap.String("event", event)}, fields...)
	FromContext(ctx).WithOptions(zap.AddCallerSkip(1)).Info(event, f...)
}

func FieldString(ctx context.Context, key string) string {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.Fields[key].String
	}
	return ""
}

func MarkRoot(ctx context.Context) {
	if s := StateFrom(ctx); s != nil {
		s.mu.Lock()
		s.RootLogged = true
		s.mu.Unlock()
	}
}

type configurationError struct{ key, reason string }

func (e *configurationError) Error() string { return e.key + ": " + e.reason }
