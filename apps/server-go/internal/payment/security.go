package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/fixpro/server/internal/platform/httpx"
	"strconv"
	"strings"
	"time"
)

type ticket struct {
	PaymentID  int64  `json:"paymentId"`
	OrgID      int64  `json:"orgId"`
	CustomerID int64  `json:"customerId"`
	Amount     int64  `json:"amount"`
	AppID      string `json:"appId"`
	Expires    int64  `json:"expires"`
}

func mac(secret, message string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
func signTicket(t ticket, secret string) string {
	b, _ := json.Marshal(t)
	p := base64.RawURLEncoding.EncodeToString(b)
	return p + "." + mac(secret, p)
}
func readTicket(value, secret string) (ticket, error) {
	var t ticket
	parts := strings.Split(value, ".")
	if len(secret) < 32 || len(parts) != 2 || !hmac.Equal([]byte(mac(secret, parts[0])), []byte(parts[1])) {
		return t, httpx.E("PAYMENT_TICKET_INVALID", "支付凭据无效", 401)
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return t, e
	}
	if e = json.Unmarshal(b, &t); e != nil {
		return t, e
	}
	if t.Expires <= time.Now().Unix() {
		return t, httpx.E("PAYMENT_TICKET_EXPIRED", "支付凭据已过期，请重试", 401)
	}
	return t, nil
}
func signature(secret, method, path, stamp, nonce string, body []byte) string {
	digest := sha256.Sum256(body)
	return mac(secret, method+"\n"+path+"\n"+stamp+"\n"+nonce+"\n"+hex.EncodeToString(digest[:]))
}
func validSignature(secret, method, path, stamp, nonce, sig string, body []byte) bool {
	ts, e := strconv.ParseInt(stamp, 10, 64)
	now := time.Now().Unix()
	return len(secret) >= 32 && e == nil && ts >= now-300 && ts <= now+300 && len(nonce) >= 16 && len(nonce) <= 128 && hmac.Equal([]byte(signature(secret, method, path, stamp, nonce, body)), []byte(sig))
}
