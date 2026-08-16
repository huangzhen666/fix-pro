package workforce

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type AuthHandler struct {
	service *Service
	store   *auth.WorkerSessionStore
}

func NewAuthHandler(service *Service, store *auth.WorkerSessionStore) *AuthHandler {
	return &AuthHandler{service: service, store: store}
}

type workerLoginRequest struct {
	OrgID    int64  `json:"orgId"`
	Mobile   string `json:"mobile"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in workerLoginRequest
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if !validWorkerMobile(in.Mobile) {
		httpx.Failure(w, r, httpx.E("WORKER_MOBILE_INVALID", "手机号格式不正确", http.StatusBadRequest))
		return
	}
	result, err := h.store.Login(r.Context(), in.OrgID, in.Mobile, in.Password, r.UserAgent(), workerClientIP(r))
	if errors.Is(err, auth.ErrWorkerLoginFailed) {
		httpx.Failure(w, r, httpx.E("WORKER_LOGIN_FAILED", "手机号或密码错误，或账号暂不可用", http.StatusUnauthorized))
		return
	}
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]any{
		"token":              result.Token,
		"expiresAt":          result.ExpiresAt,
		"mustChangePassword": result.MustChangePassword,
		"worker": map[string]any{
			"id":          result.Principal.SubjectID,
			"workerNo":    result.WorkerNo,
			"displayName": result.Principal.Name,
			"mobile":      result.Mobile,
		},
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.From(r.Context())
	if !ok || p.Role != "WORKER" {
		httpx.Failure(w, r, httpx.E("WORKER_SESSION_INVALID", "需要师傅登录", http.StatusUnauthorized))
		return
	}
	profile, err := h.service.worker(r.Context(), p, p.SubjectID)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]any{
		"worker":             profile,
		"mustChangePassword": p.MustChangePassword,
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.store.Logout(r.Context(), workerToken(r))
	httpx.Success(w, r, map[string]bool{"loggedOut": true})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	p, ok := auth.From(r.Context())
	if !ok || p.Role != "WORKER" {
		httpx.Failure(w, r, httpx.E("WORKER_SESSION_INVALID", "需要师傅登录", http.StatusUnauthorized))
		return
	}
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
		ConfirmPassword string `json:"confirmPassword"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if in.NewPassword != in.ConfirmPassword {
		httpx.Failure(w, r, httpx.E("WORKER_PASSWORD_CONFIRM_MISMATCH", "两次输入的新密码不一致", http.StatusBadRequest))
		return
	}
	err := h.store.ChangePassword(r.Context(), p, workerToken(r), in.CurrentPassword, in.NewPassword)
	switch {
	case errors.Is(err, auth.ErrWorkerCurrentPassword):
		httpx.Failure(w, r, httpx.E("WORKER_CURRENT_PASSWORD_INVALID", "当前密码错误", http.StatusBadRequest))
	case errors.Is(err, auth.ErrWorkerPasswordTooWeak):
		httpx.Failure(w, r, httpx.E("WORKER_PASSWORD_WEAK", "新密码至少 12 位，且必须包含字母和数字，不能使用初始密码", http.StatusBadRequest))
	case err != nil:
		httpx.Failure(w, r, err)
	default:
		httpx.Success(w, r, map[string]bool{"changed": true})
	}
}

func workerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get(auth.WorkerSessionHeader))
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func validWorkerMobile(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 11 || value[0] != '1' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func workerClientIP(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); value != "" {
		return strings.Split(value, ",")[0]
	}
	return strings.TrimSpace(strings.Split(r.RemoteAddr, ":")[0])
}
