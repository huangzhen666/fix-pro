package payment

import (
	"context"
	"github.com/fixpro/server/internal/platform/config"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestInternalEventsRequireFreshRouteBoundSignature(t *testing.T) {
	secret := strings.Repeat("s", 32)
	h := NewHandler(New(nil, config.Payment{WechatEnabled: true, BridgeSecret: secret}, "staging"))
	for _, mode := range []string{"unsigned", "wrong-route", "stale", "signed-mock"} {
		body := `{"kind":"PAYMENT","channel":"MOCK"}`
		r := httptest.NewRequest("POST", "/api/v1/internal/payment/events", strings.NewReader(body))
		r.SetPathValue("action", "events")
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		n := strings.Repeat("n", 20)
		path := r.URL.Path
		if mode == "wrong-route" {
			path += "/other"
		}
		if mode == "stale" {
			stamp = strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
		}
		if mode != "unsigned" {
			r.Header.Set("X-Pay-Timestamp", stamp)
			r.Header.Set("X-Pay-Nonce", n)
			r.Header.Set("X-Pay-Signature", signature(secret, "POST", path, stamp, n, []byte(body)))
		}
		w := httptest.NewRecorder()
		h.Internal(w, r)
		expected := 401
		if mode == "signed-mock" {
			expected = 403
		}
		if w.Code != expected {
			t.Fatalf("%s: %d %s", mode, w.Code, w.Body.String())
		}
	}
}
func TestMockRequestRequiresLoopbackWithoutForwarding(t *testing.T) {
	for _, tc := range []struct {
		remote, forwarded string
		ok                bool
	}{{"127.0.0.1:1234", "", true}, {"[::1]:1234", "", true}, {"10.0.0.1:1234", "", false}, {"127.0.0.1:1234", "10.0.0.1", false}} {
		r := httptest.NewRequest("POST", "/mock", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if localRequest(r) != tc.ok {
			t.Fatalf("%+v", tc)
		}
	}
}
func TestCollectionPauseRetainsProcessingButBlocksNewTickets(t *testing.T) {
	s := New(nil, config.Payment{WechatEnabled: true, CollectionPaused: true}, "staging")
	if _, e := s.Authorize(context.Background(), "ticket"); e == nil {
		t.Fatal("paused authorization accepted")
	}
	if !s.channelAllowed("WECHAT") {
		t.Fatal("pause must not disable refunds/reconciliation")
	}
}
