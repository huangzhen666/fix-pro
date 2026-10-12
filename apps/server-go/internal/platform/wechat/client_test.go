package wechat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGatewayCodesAndTokenCache(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/stable_token":
			calls.Add(1)
			w.Write([]byte(`{"access_token":"token","expires_in":7200}`))
		case "/sns/jscode2session":
			if r.URL.Query().Get("js_code") != "login-code" {
				t.Error("wrong login code")
			}
			w.Write([]byte(`{"openid":"openid"}`))
		case "/wxa/business/getuserphonenumber":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["code"] != "phone-code" {
				t.Error("wrong phone code")
			}
			w.Write([]byte(`{"phone_info":{"purePhoneNumber":"13800138000","countryCode":"86","watermark":{"appid":"app"}}}`))
		case "/wxa/msg_sec_check":
			w.Write([]byte(`{"errcode":0,"result":{"suggest":"pass"}}`))
		default:
			t.Error(r.URL.Path)
		}
	}))
	defer srv.Close()
	c := New("app", "secret")
	c.BaseURL = srv.URL
	ctx := context.Background()
	id, e := c.Identity(ctx, "login-code")
	if e != nil || id.OpenID != "openid" {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, e := c.Phone(ctx, "phone-code")
			if e != nil || p.Country != "86" {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("token refresh not coalesced", calls.Load())
	}
	if e = c.CheckNickname(ctx, "openid", "昵称"); e != nil {
		t.Fatal(e)
	}
}
func TestGatewayFailureSanitized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.Write([]byte(`{"errcode":40029,"errmsg":"private secret"}`))
	}))
	defer srv.Close()
	c := New("app", "private-secret")
	c.BaseURL = srv.URL
	c.HTTP.Timeout = time.Millisecond
	if _, e := c.Identity(context.Background(), "private-code"); e == nil {
		t.Fatal("timeout accepted")
	}
	c.HTTP.Timeout = time.Second
	if _, e := c.Identity(context.Background(), "private-code"); e == nil {
		t.Fatal("invalid code accepted")
	}
}

func TestPhoneFailuresAndControlledRefresh(t *testing.T) {
	for _, mode := range []string{"expired-token", "bad-code", "quota", "wrong-app"} {
		t.Run(mode, func(t *testing.T) {
			var tokens atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/cgi-bin/stable_token" {
					n := tokens.Add(1)
					if n == 1 {
						w.Write([]byte(`{"access_token":"old","expires_in":7200}`))
					} else {
						w.Write([]byte(`{"access_token":"new","expires_in":7200}`))
					}
					return
				}
				switch mode {
				case "expired-token":
					if r.URL.Query().Get("access_token") == "old" {
						w.Write([]byte(`{"errcode":40001}`))
						return
					}
				case "bad-code":
					w.Write([]byte(`{"errcode":40029}`))
					return
				case "quota":
					w.Write([]byte(`{"errcode":200041}`))
					return
				case "wrong-app":
					w.Write([]byte(`{"phone_info":{"purePhoneNumber":"13800138000","countryCode":"86","watermark":{"appid":"other"}}}`))
					return
				}
				w.Write([]byte(`{"phone_info":{"purePhoneNumber":"13800138000","countryCode":"86","watermark":{"appid":"app"}}}`))
			}))
			defer srv.Close()
			c := New("app", "secret")
			c.BaseURL = srv.URL
			_, e := c.Phone(context.Background(), "phone-code")
			if mode == "expired-token" {
				if e != nil || tokens.Load() != 2 {
					t.Fatal("refresh failed", e, tokens.Load())
				}
			} else if e == nil {
				t.Fatal("failure accepted")
			}
		})
	}
}
