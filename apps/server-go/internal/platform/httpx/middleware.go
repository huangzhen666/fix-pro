package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type contextKey string

const requestIDKey contextKey = "requestId"

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
func Middleware(log *slog.Logger, origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, x := range origins {
		allowed[x] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			var b [16]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if origin := r.Header.Get("Origin"); allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id, X-CSRF-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		rw := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if x := recover(); x != nil {
				log.Error("panic", "requestId", id, "panic", x, "stack", string(debug.Stack()))
				Failure(rw, r, E("INTERNAL_ERROR", "internal server error", 500))
			}
			log.Info("request", "requestId", id, "method", r.Method, "path", r.URL.Path, "status", rw.status, "durationMs", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rw, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(v int) { s.status = v; s.ResponseWriter.WriteHeader(v) }
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
