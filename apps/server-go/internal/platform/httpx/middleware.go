package httpx

import (
	"context"
	"github.com/felixge/httpsnoop"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func RequestID(ctx context.Context) string {
	if s := logging.StateFrom(ctx); s != nil {
		return s.ID
	}
	return ""
}
func Middleware(log *zap.Logger, origins []string, next http.Handler) http.Handler {
	return MiddlewareConfig(log, logging.Defaults(), origins, next)
}
func MiddlewareConfig(log *zap.Logger, c logging.Config, origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, v := range origins {
		allowed[v] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		values := r.Header.Values("X-Request-Id")
		id := ""
		if len(values) == 1 && validRequestID.MatchString(values[0]) {
			id = values[0]
		}
		if id == "" {
			id = logging.ID()
		}
		r = r.WithContext(logging.Context(r.Context(), log, c, id))
		logging.Bind(r.Context(), zap.String("principalType", "ANONYMOUS"))
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id, X-CSRF-Token, X-Customer-Interaction")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
		}
		status, committed := 200, false
		rw := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if code >= 200 && !committed {
						status = code
						committed = true
					}
					next(code)
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(b []byte) (int, error) {
					if !committed {
						status = 200
						committed = true
					}
					return next(b)
				}
			},
			Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() {
					if !committed {
						status = 200
						committed = true
					}
					next()
				}
			},
		})
		defer func() {
			if x := recover(); x != nil {
				logging.Bind(r.Context(), zap.String("module", "http"))
				logging.FromContext(r.Context()).Error("request panic", zap.String("event", "http.panic"), zap.String("errorType", "panic"), zap.String("errorMessage", "handler panicked"), zap.String("stack", logging.Stack()), zap.Bool("responseCommitted", committed))
				logging.MarkRoot(r.Context())
				logging.SetCode(r.Context(), "INTERNAL_ERROR")
				if !committed {
					Failure(rw, r, E("INTERNAL_ERROR", "internal server error", 500))
				}
			}
			route := r.Pattern
			if route == "" {
				route = "UNMATCHED"
			}
			// A route template is safe to retain; raw dynamic path/query may carry secrets.
			path := route
			if route == "UNMATCHED" {
				path = "UNMATCHED"
			}
			code := logging.Code(r.Context())
			if code == "OK" && status >= 400 {
				code = http.StatusText(status)
			}
			duration := time.Since(start)
			threshold := c.HTTPSlow
			if strings.Contains(route, "media") || strings.Contains(route, "avatar") {
				threshold = c.MediaSlow
			}
			fields := []zap.Field{zap.String("module", "http"), zap.String("event", "http.request.completed"), zap.String("method", r.Method), zap.String("route", route), zap.String("path", path), zap.Int("status", status), zap.String("errorCode", code), zap.Int64("durationMs", duration.Milliseconds())}
			l := logging.FromContext(r.Context())
			if status >= 500 {
				l.Error("request completed", fields...)
			} else if status >= 400 || duration >= threshold {
				l.Warn("request completed", fields...)
			} else {
				l.Info("request completed", fields...)
			}
		}()
		if r.Method == http.MethodOptions {
			rw.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(rw, r)
	})
}

func PathID(r *http.Request, name string) (int64, error) {
	v := strings.TrimSpace(r.PathValue(name))
	var n int64
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0, E("VALIDATION_ERROR", "ID 格式错误", 400)
		}
		n = n*10 + int64(c-'0')
	}
	if n <= 0 {
		return 0, E("VALIDATION_ERROR", "ID 格式错误", 400)
	}
	return n, nil
}
