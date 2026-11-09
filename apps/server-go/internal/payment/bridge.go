package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/fixpro/server/internal/platform/config"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type bridge struct {
	cfg    config.Payment
	client *http.Client
}

func newBridge(c config.Payment) *bridge {
	return &bridge{c, &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (b *bridge) call(ctx context.Context, action string, data any) (Event, error) {
	var out Event
	raw, e := json.Marshal(data)
	if e != nil {
		return out, e
	}
	endpoint := strings.TrimRight(b.cfg.BridgeURL, "/") + "/internal/" + action
	req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(raw))
	if e != nil {
		return out, e
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomID("N")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pay-Timestamp", stamp)
	req.Header.Set("X-Pay-Nonce", nonce)
	req.Header.Set("X-Pay-Signature", signature(b.cfg.BridgeSecret, "POST", req.URL.Path, stamp, nonce, raw))
	resp, e := b.client.Do(req)
	if e != nil {
		return out, fmt.Errorf("payment bridge unavailable")
	}
	defer resp.Body.Close()
	body, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if e != nil {
		return out, e
	}
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("payment bridge returned HTTP %d", resp.StatusCode)
	}
	if !validSignature(b.cfg.BridgeSecret, "RESPONSE", req.URL.Path, resp.Header.Get("X-Pay-Timestamp"), nonce, resp.Header.Get("X-Pay-Signature"), body) {
		return out, fmt.Errorf("payment bridge response signature invalid")
	}
	e = json.Unmarshal(body, &out)
	return out, e
}
