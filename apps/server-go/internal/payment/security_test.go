package payment

import (
	"github.com/fixpro/server/internal/platform/config"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTicketAndBridgeSignatures(t *testing.T) {
	secret := strings.Repeat("a", 32)
	v := ticket{PaymentID: 1, OrgID: 2, CustomerID: 3, Amount: 100, AppID: "app", Expires: time.Now().Add(time.Minute).Unix()}
	token := signTicket(v, secret)
	if got, e := readTicket(token, secret); e != nil || got != v {
		t.Fatalf("ticket %v %v", got, e)
	}
	if _, e := readTicket(token+"0", secret); e == nil {
		t.Fatal("tampered ticket accepted")
	}
	v.Expires = time.Now().Add(-time.Minute).Unix()
	if _, e := readTicket(signTicket(v, secret), secret); e == nil {
		t.Fatal("expired ticket accepted")
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	n := strings.Repeat("b", 20)
	body := []byte(`{"amount":100}`)
	sig := signature(secret, "POST", "/events", stamp, n, body)
	if !validSignature(secret, "POST", "/events", stamp, n, sig, body) {
		t.Fatal("valid signature rejected")
	}
	for _, path := range []string{"/other", "/events?changed=1"} {
		if validSignature(secret, "POST", path, stamp, n, sig, body) {
			t.Fatal("path tampering accepted")
		}
	}
	if validSignature(secret, "POST", "/events", stamp, n, sig, []byte(`{"amount":1}`)) {
		t.Fatal("body tampering accepted")
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	if validSignature(secret, "POST", "/events", old, n, signature(secret, "POST", "/events", old, n, body), body) {
		t.Fatal("stale signed request accepted")
	}
}
func TestChannelEventsCannotBypassEnvironment(t *testing.T) {
	c := config.Payment{MockEnabled: true}
	e := Event{Kind: "PAYMENT", Channel: "MOCK", OutTradeNo: "order", TransactionID: "MOCK_id", Currency: "CNY", Amount: 100, Total: 100, Status: "SUCCESS"}
	if validateEvent(e, c, "production") == nil {
		t.Fatal("production accepted mock")
	}
	if validateEvent(e, c, "local") != nil {
		t.Fatal("local rejected mock")
	}
	e.Channel = "WECHAT"
	c.WechatEnabled = true
	c.AppID = "app"
	c.MerchantID = "merchant"
	if validateEvent(e, c, "production") == nil {
		t.Fatal("missing merchant binding accepted")
	}
	e.AppID = "app"
	e.MerchantID = "merchant"
	if validateEvent(e, c, "production") != nil {
		t.Fatal("valid identity rejected")
	}
}
