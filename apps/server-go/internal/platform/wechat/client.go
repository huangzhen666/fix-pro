package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Identity struct {
	OpenID  string `json:"openid"`
	UnionID string `json:"unionid"`
}
type Phone struct {
	Number    string `json:"purePhoneNumber"`
	Country   string `json:"countryCode"`
	Watermark struct {
		AppID string `json:"appid"`
	} `json:"watermark"`
}
type Gateway interface {
	Identity(context.Context, string) (Identity, error)
	Phone(context.Context, string) (Phone, error)
	CheckNickname(context.Context, string, string) error
}
type Client struct {
	AppID, Secret string
	BaseURL       string
	HTTP          *http.Client
	mu            sync.Mutex
	token         string
	expires       time.Time
}

func New(appid, secret string) *Client {
	return &Client{AppID: appid, Secret: secret, BaseURL: "https://api.weixin.qq.com", HTTP: &http.Client{Timeout: 5 * time.Second}}
}
func unavailable() error {
	return httpx.E("PHONE_LOGIN_UNAVAILABLE", "微信手机号服务暂不可用，请稍后重试", 503)
}
func (c *Client) call(ctx context.Context, method, path string, body any, dst any) (code int, err error) {
	start := time.Now()
	operation := strings.Split(path, "?")[0]
	status := 0
	defer func() {
		result := "success"
		if err != nil || code != 0 {
			result = "failure"
		}
		fields := []zap.Field{zap.String("module", "wechat"), zap.String("event", "dependency.attempt.completed"), zap.String("dependency", "wechat"), zap.String("operation", operation), zap.String("result", result), zap.Int("status", status), zap.Int("upstreamCode", code), zap.Int64("durationMs", time.Since(start).Milliseconds())}
		threshold := logging.Defaults().UpstreamSlow
		if state := logging.StateFrom(ctx); state != nil {
			threshold = state.Config.UpstreamSlow
		}
		if err != nil {
			logging.QueryFailure(ctx, "wechat"+operation, "")
			fields = append(fields, logging.SafeError(err)...)
		}
		if err != nil || code != 0 || time.Since(start) >= threshold {
			logging.FromContext(ctx).Warn("dependency attempt completed", fields...)
		} else {
			logging.FromContext(ctx).Info("dependency attempt completed", fields...)
		}
	}()
	if c.AppID == "" || c.Secret == "" {
		return 0, unavailable()
	}
	var data []byte
	if body != nil {
		var e error
		data, e = json.Marshal(body)
		if e != nil {
			return 0, unavailable()
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(data))
	if e != nil {
		return 0, unavailable()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := c.HTTP.Do(req)
	if e != nil {
		x := unavailable().(*httpx.Error)
		x.Cause = e
		return 0, x
	}
	status = resp.StatusCode
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, unavailable()
	}
	data, e = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if e != nil {
		return 0, unavailable()
	}
	var result struct {
		ErrCode int `json:"errcode"`
	}
	if json.Unmarshal(data, &result) != nil {
		return 0, unavailable()
	}
	if result.ErrCode != 0 {
		return result.ErrCode, nil
	}
	if json.Unmarshal(data, dst) != nil {
		return 0, unavailable()
	}
	return 0, nil
}
func (c *Client) Identity(ctx context.Context, code string) (Identity, error) {
	var out Identity
	q := url.Values{"appid": {c.AppID}, "secret": {c.Secret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	n, e := c.call(ctx, "GET", "/sns/jscode2session?"+q.Encode(), nil, &out)
	if e != nil {
		x := httpx.E("WECHAT_LOGIN_UNAVAILABLE", "微信登录校验暂不可用，请重试", 503)
		x.Cause = e
		return out, x
	}
	if n != 0 || out.OpenID == "" {
		return out, httpx.E("WECHAT_LOGIN_FAILED", "微信登录凭证已失效，请重新授权", 401)
	}
	return out, nil
}
func (c *Client) accessToken(ctx context.Context, observed string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) && (observed == "" || c.token != observed) {
		return c.token, nil
	}
	var out struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	n, e := c.call(ctx, "POST", "/cgi-bin/stable_token", map[string]any{"grant_type": "client_credential", "appid": c.AppID, "secret": c.Secret, "force_refresh": observed != ""}, &out)
	if e != nil {
		return "", e
	}
	if n != 0 || out.Token == "" || out.Expires <= 120 {
		return "", unavailable()
	}
	c.token = out.Token
	c.expires = time.Now().Add(time.Duration(out.Expires-120) * time.Second)
	return c.token, nil
}
func (c *Client) authorized(ctx context.Context, path string, body, dst any) (int, error) {
	token, e := c.accessToken(ctx, "")
	if e != nil {
		return 0, e
	}
	n, e := c.call(ctx, "POST", path+"?access_token="+url.QueryEscape(token), body, dst)
	if e == nil && (n == 40001 || n == 42001 || n == 40014) {
		token, e = c.accessToken(ctx, token)
		if e != nil {
			return 0, e
		}
		n, e = c.call(ctx, "POST", path+"?access_token="+url.QueryEscape(token), body, dst)
	}
	return n, e
}
func (c *Client) Phone(ctx context.Context, code string) (Phone, error) {
	var out struct {
		Phone Phone `json:"phone_info"`
	}
	n, e := c.authorized(ctx, "/wxa/business/getuserphonenumber", map[string]string{"code": code}, &out)
	if e != nil {
		return out.Phone, e
	}
	if n == 40029 || n == 40163 {
		return out.Phone, httpx.E("PHONE_LOGIN_FAILED", "手机号授权凭证已失效，请重新授权", 401)
	}
	if n != 0 {
		return out.Phone, unavailable()
	}
	if out.Phone.Watermark.AppID != c.AppID {
		return out.Phone, httpx.E("PHONE_LOGIN_FAILED", "手机号授权验证失败", 401)
	}
	return out.Phone, nil
}
func (c *Client) CheckNickname(ctx context.Context, openid, name string) error {
	var out struct {
		Result struct {
			Suggest string `json:"suggest"`
		} `json:"result"`
	}
	n, e := c.authorized(ctx, "/wxa/msg_sec_check", map[string]any{"openid": openid, "scene": 1, "version": 2, "content": name}, &out)
	if e != nil {
		return httpx.E("PROFILE_CHECK_UNAVAILABLE", "昵称检测暂不可用，请重试", 503)
	}
	if n != 0 || out.Result.Suggest != "pass" {
		if n == 87014 || out.Result.Suggest == "risky" || out.Result.Suggest == "review" {
			return httpx.E("VALIDATION_ERROR", "昵称未通过检测，请修改", 400)
		}
		return httpx.E("PROFILE_CHECK_UNAVAILABLE", "昵称检测暂不可用，请重试", 503)
	}
	return nil
}
