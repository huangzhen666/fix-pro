package config

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

type Payment struct {
	MockEnabled      bool
	WechatEnabled    bool
	CollectionPaused bool
	OrderTTL         time.Duration
	AppID            string
	MerchantID       string
	BridgeURL        string
	BridgeSecret     string
	TicketSecret     string
	CloudEnv         string
	CloudFunction    string
}

func (p Payment) Validate(env, addr string) error {
	if p.OrderTTL <= 0 || p.OrderTTL > 24*time.Hour {
		return errors.New("PAYMENT_ORDER_TTL must be positive and <=24h")
	}
	if p.MockEnabled {
		host, _, err := net.SplitHostPort(addr)
		if env != "local" || err != nil || (host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback())) {
			return errors.New("mock payment requires APP_ENV=local and a loopback HTTP_ADDR")
		}
	}
	if p.WechatEnabled {
		u, e := url.Parse(p.BridgeURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return errors.New("PAYMENT_BRIDGE_URL must be an HTTPS root URL without credentials/query/fragment")
		}
		if p.AppID == "" || p.MerchantID == "" || p.CloudEnv == "" || p.CloudFunction == "" || len(p.BridgeSecret) < 32 || len(p.TicketSecret) < 32 || strings.TrimSpace(p.BridgeSecret) == strings.TrimSpace(p.TicketSecret) {
			return errors.New("wechat payment requires app/merchant/cloud configuration and distinct secrets of at least 32 bytes")
		}
	}
	return nil
}
