package payment

import (
	"encoding/json"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func orderID(r *http.Request) (int64, error) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id <= 0 {
		return 0, httpx.E("VALIDATION_ERROR", "订单编号无效", 400)
	}
	return id, nil
}
func reply(w http.ResponseWriter, r *http.Request, value any, e error) {
	if e != nil {
		httpx.Failure(w, r, e)
	} else {
		httpx.Success(w, r, value)
	}
}
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := orderID(r)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	v, e := h.s.Overview(r.Context(), p, id, strings.HasPrefix(r.URL.Path, "/api/v1/admin/"))
	reply(w, r, v, e)
}
func (h *Handler) Prepare(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := orderID(r)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	var body struct {
		Channel string `json:"channel"`
	}
	if e = httpx.DecodeJSON(w, r, &body); e != nil {
		reply(w, r, nil, e)
		return
	}
	if body.Channel == "MOCK" && !localRequest(r) {
		reply(w, r, nil, httpx.E("FORBIDDEN", "模拟支付只允许本地直连", 403))
		return
	}
	v, e := h.s.Prepare(r.Context(), p, id, body.Channel)
	reply(w, r, v, e)
}
func localRequest(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return e == nil && ip != nil && ip.IsLoopback() && r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-For") == ""
}
func (h *Handler) Mock(w http.ResponseWriter, r *http.Request) {
	if !localRequest(r) {
		reply(w, r, nil, httpx.E("FORBIDDEN", "模拟支付只允许本地直连", 403))
		return
	}
	p, _ := auth.From(r.Context())
	id, e := orderID(r)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	var body struct {
		Outcome string `json:"outcome"`
	}
	if e = httpx.DecodeJSON(w, r, &body); e != nil {
		reply(w, r, nil, e)
		return
	}
	e = h.s.Mock(r.Context(), p, id, body.Outcome)
	reply(w, r, map[string]bool{"accepted": e == nil}, e)
}
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := orderID(r)
	if e != nil {
		reply(w, r, nil, e)
		return
	}
	var body struct {
		Amount int64  `json:"amount"`
		Reason string `json:"reason"`
	}
	if e = httpx.DecodeJSON(w, r, &body); e != nil {
		reply(w, r, nil, e)
		return
	}
	v, e := h.s.Refund(r.Context(), p, id, body.Amount, strings.TrimSpace(body.Reason), r.Header.Get("Idempotency-Key"))
	reply(w, r, v, e)
}

// Internal accepts only bridge-authenticated bodies. No user-facing payment status mutation exists.
func (h *Handler) Internal(w http.ResponseWriter, r *http.Request) {
	if !h.s.cfg.WechatEnabled {
		reply(w, r, nil, httpx.E("PAYMENT_DISABLED", "真实支付未启用", 404))
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if e != nil {
		reply(w, r, nil, httpx.E("VALIDATION_ERROR", "请求过大", 400))
		return
	}
	if !validSignature(h.s.cfg.BridgeSecret, r.Method, r.URL.Path, r.Header.Get("X-Pay-Timestamp"), r.Header.Get("X-Pay-Nonce"), r.Header.Get("X-Pay-Signature"), raw) {
		reply(w, r, nil, httpx.E("UNAUTHORIZED", "服务签名无效", 401))
		return
	}
	switch r.PathValue("action") {
	case "authorize":
		var body struct {
			Ticket string `json:"ticket"`
		}
		if e = json.Unmarshal(raw, &body); e != nil {
			reply(w, r, nil, httpx.E("VALIDATION_ERROR", "参数错误", 400))
			return
		}
		v, e := h.s.Authorize(r.Context(), body.Ticket)
		reply(w, r, v, e)
	case "prepay":
		var body struct {
			Ticket   string `json:"ticket"`
			PrepayID string `json:"prepayId"`
		}
		if e = json.Unmarshal(raw, &body); e != nil {
			reply(w, r, nil, httpx.E("VALIDATION_ERROR", "参数错误", 400))
			return
		}
		e = h.s.SavePrepay(r.Context(), body.Ticket, body.PrepayID)
		reply(w, r, map[string]bool{"saved": e == nil}, e)
	case "events":
		var ev Event
		if e = json.Unmarshal(raw, &ev); e != nil {
			reply(w, r, nil, httpx.E("VALIDATION_ERROR", "参数错误", 400))
			return
		}
		if ev.Channel != "WECHAT" {
			reply(w, r, nil, httpx.E("FORBIDDEN", "仅接受真实渠道事件", 403))
			return
		}
		e = h.s.Apply(r.Context(), ev)
		reply(w, r, map[string]bool{"accepted": e == nil}, e)
	default:
		reply(w, r, nil, httpx.E("NOT_FOUND", "接口不存在", 404))
	}
}
