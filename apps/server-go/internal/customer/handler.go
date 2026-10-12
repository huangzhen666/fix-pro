package customer

import (
	"bytes"
	"encoding/json"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net"
	"net/http"
	"sync"
	"time"
)

type Handler struct {
	s        *Service
	mu       sync.Mutex
	attempts map[string]bucket
}
type bucket struct {
	since time.Time
	count int
}

func NewHandler(s *Service) *Handler { return &Handler{s: s, attempts: make(map[string]bucket)} }
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	h.mu.Lock()
	now := time.Now()
	b := h.attempts[ip]
	if now.Sub(b.since) > time.Minute {
		b = bucket{since: now}
	}
	b.count++
	if len(h.attempts) > 10000 {
		for k, v := range h.attempts {
			if now.Sub(v.since) > time.Minute {
				delete(h.attempts, k)
			}
		}
	}
	h.attempts[ip] = b
	h.mu.Unlock()
	if b.count > 20 {
		httpx.Failure(w, r, httpx.E("RATE_LIMITED", "操作频繁，请稍后再试", 429))
		return
	}
	var in struct {
		PhoneCode string `json:"phoneCode"`
		LoginCode string `json:"loginCode"`
	}
	if e := httpx.DecodeJSON(w, r, &in); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.s.Login(r.Context(), in.PhoneCode, in.LoginCode)
	respond(w, r, out, e)
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	s, e := h.s.Session(r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	p, e := h.s.Profile(r.Context(), s.Principal.OrgID, s.Principal.SubjectID)
	respond(w, r, map[string]any{"customer": p, "expiresAt": s.ExpiresAt}, e)
}
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	s, e := h.s.Session(r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var in struct {
		LoginCode string `json:"loginCode"`
	}
	if e = httpx.DecodeJSON(w, r, &in); e != nil || in.LoginCode == "" {
		if e == nil {
			e = httpx.E("VALIDATION_ERROR", "缺少微信登录凭证", 400)
		}
		respond(w, r, nil, e)
		return
	}
	id, e := h.s.Gateway.Identity(r.Context(), in.LoginCode)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	if id.OpenID != s.OpenID || s.AppID != h.s.AppID {
		respond(w, r, nil, httpx.E("CUSTOMER_WECHAT_CONTEXT_CHANGED", "登录环境已变化，请重新授权手机号", 401))
		return
	}
	h.Me(w, r)
}
func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	out, e := h.s.Profile(r.Context(), p.OrgID, p.SubjectID)
	respond(w, r, out, e)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	s, e := h.s.Session(r)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	var body struct {
		DisplayName   string          `json:"displayName"`
		AvatarMediaID json.RawMessage `json:"avatarMediaId"`
		Version       *int            `json:"version"`
	}
	if e = httpx.DecodeJSON(w, r, &body); e != nil {
		respond(w, r, nil, e)
		return
	}
	in := ProfileWrite{DisplayName: body.DisplayName, Version: body.Version}
	if len(body.AvatarMediaID) == 0 {
		respond(w, r, nil, httpx.E("VALIDATION_ERROR", "缺少头像字段", 400))
		return
	}
	if !bytes.Equal(bytes.TrimSpace(body.AvatarMediaID), []byte("null")) {
		var id string
		if json.Unmarshal(body.AvatarMediaID, &id) != nil {
			respond(w, r, nil, httpx.E("VALIDATION_ERROR", "头像编号无效", 400))
			return
		}
		in.AvatarMediaID = &id
	}
	out, e := h.s.UpdateProfile(r.Context(), s, in)
	respond(w, r, out, e)
}
func (h *Handler) Contact(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	if h.s.Codec == nil {
		respond(w, r, nil, invalid())
		return
	}
	var encrypted []byte
	e := h.s.DB.QueryRowContext(r.Context(), `SELECT mobile_cipher FROM customer WHERE org_id=$1 AND id=$2`, p.OrgID, p.SubjectID).Scan(&encrypted)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	mobile, e := h.s.Codec.Decrypt(encrypted)
	if e != nil {
		respond(w, r, nil, e)
		return
	}
	respond(w, r, map[string]string{"contactMobile": mobile[3:]}, nil)
}
func respond(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		httpx.Failure(w, r, e)
	} else {
		httpx.Success(w, r, v)
	}
}
