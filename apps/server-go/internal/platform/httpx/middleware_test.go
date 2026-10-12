package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func logLines(t *testing.T, b *bytes.Buffer) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if e := json.Unmarshal([]byte(line), &m); e != nil {
			t.Fatal(e)
		}
		out = append(out, m)
	}
	return out
}
func TestRequestIDsAndFinalStatus(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		ids          []string
		valid        bool
		status       int
	}{{"valid", "GET", []string{"uuid-test_1"}, true, 201}, {"bad", "GET", []string{"bad,id"}, false, 201}, {"many", "GET", []string{"one", "two"}, false, 201}, {"options", "OPTIONS", nil, false, 204}} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			l, _ := logging.New(logging.Defaults(), &b)
			h := Middleware(l, []string{"https://admin.example"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(103)
				w.WriteHeader(201)
				w.WriteHeader(500)
			}))
			r := httptest.NewRequest(tc.method, "/api?phone=secret", nil)
			r.Header["X-Request-Id"] = tc.ids
			r.Header.Set("Origin", "https://admin.example")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			id := w.Header().Get("X-Request-Id")
			if !validRequestID.MatchString(id) {
				t.Fatal("invalid response ID")
			}
			if tc.valid && id != tc.ids[0] {
				t.Fatal("ID changed")
			}
			if w.Header().Get("Access-Control-Expose-Headers") != "X-Request-Id" {
				t.Fatal("missing CORS expose")
			}
			lines := logLines(t, &b)
			if len(lines) != 1 || int(lines[0]["status"].(float64)) != tc.status {
				t.Fatalf("status %v", lines)
			}
			if strings.Contains(b.String(), "phone") {
				t.Fatal("query leaked")
			}
		})
	}
}
func TestRootAndPanicBoundaries(t *testing.T) {
	for _, kind := range []string{"root", "panic_before", "panic_after"} {
		t.Run(kind, func(t *testing.T) {
			var b bytes.Buffer
			l, _ := logging.New(logging.Defaults(), &b)
			h := Middleware(l, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "root":
					e := E("INTERNAL_ERROR", "internal server error", 500)
					e.Cause = errors.New("secret-sentinel")
					Failure(w, r, e)
				case "panic_after":
					w.WriteHeader(202)
					panic("secret-sentinel")
				default:
					panic("secret-sentinel")
				}
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
			if strings.Contains(b.String(), "secret-sentinel") || strings.Contains(w.Body.String(), "secret-sentinel") {
				t.Fatal("panic/error leaked")
			}
			if kind == "panic_after" && (w.Code != 202 || w.Body.Len() != 0) {
				t.Fatal("committed response overwritten")
			}
			lines := logLines(t, &b)
			last := lines[len(lines)-1]
			if last["errorCode"] != "INTERNAL_ERROR" {
				t.Fatal("missing error code")
			}
			if kind == "root" && len(lines) != 2 {
				t.Fatal("expected root and completion")
			}
		})
	}
}
func TestMiddlewarePreservesWriterCapabilities(t *testing.T) {
	var b bytes.Buffer
	l, _ := logging.New(logging.Defaults(), &b)
	h := Middleware(l, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("lost Flusher")
			return
		}
		f.Flush()
		w.Write([]byte("stream"))
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !w.Flushed || w.Body.String() != "stream" {
		t.Fatal("stream failed")
	}
}
func TestConcurrentIdentityIsolation(t *testing.T) {
	var b bytes.Buffer
	l, _ := logging.New(logging.Defaults(), &b)
	h := Middleware(l, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logging.Bind(r.Context(), zap.String("principalId", r.Header.Get("Test-Principal")))
		Success(w, r, nil)
	}))
	var wg sync.WaitGroup
	for _, id := range []string{"1", "2", "3", "4"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Test-Principal", id)
			r.Header.Set("X-Request-Id", "req-"+id)
			h.ServeHTTP(httptest.NewRecorder(), r)
		}(id)
	}
	wg.Wait()
	for _, m := range logLines(t, &b) {
		if m["requestId"] != "req-"+m["principalId"].(string) {
			t.Fatal("identity crossed requests")
		}
	}
}
func TestUnwrapCause(t *testing.T) {
	e := E("X", "x", 500)
	e.Cause = context.DeadlineExceeded
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal("cause lost")
	}
}

func TestUnmatchedAndMethodDenied(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		var b bytes.Buffer
		l, _ := logging.New(logging.Defaults(), &b)
		mux := http.NewServeMux()
		mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) { Success(w, r, nil) })
		path := "/missing"
		want := 404
		if method == "POST" {
			path = "/known"
			want = 405
		}
		w := httptest.NewRecorder()
		Middleware(l, nil, mux).ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want || w.Header().Get("X-Request-Id") == "" {
			t.Fatal("missing rejection diagnostic")
		}
		if len(logLines(t, &b)) != 1 {
			t.Fatal("expected one completed event")
		}
	}
}
