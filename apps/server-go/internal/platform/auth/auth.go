package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/fixpro/server/internal/platform/httpx"
)

type Principal struct {
	OrgID                int64  `json:"orgId"`
	SubjectID            int64  `json:"subjectId"`
	AdminUserID          int64  `json:"adminUserId"`
	SessionID            int64  `json:"sessionId"`
	Role                 string `json:"role"`
	Name                 string `json:"name"`
	SessionAuthenticated bool   `json:"sessionAuthenticated"`
	PlatformSuperAdmin   bool   `json:"platformSuperAdmin"`
	LegacyBasic          bool   `json:"legacyBasic,omitempty"`
	MustChangePassword   bool   `json:"mustChangePassword"`
}

// AdminSessionOrBasic keeps a local-only compatibility path while the frontend
// migrates to cookie sessions. The compatibility path is deliberately marked on
// Principal so authorization checks can be audited and removed after cutover.
func AdminSessionOrBasic(env string, sessions *SessionStore, username, password string, allowBasic bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sessions != nil {
			if p, _, err := sessions.Authenticate(r); err == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
				return
			}
		}
		if allowBasic && env == "local" {
			u, p, ok := r.BasicAuth()
			if ok && subtle.ConstantTimeCompare([]byte(u), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1 {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, Principal{OrgID: 1, Role: "ADMIN", Name: u, LegacyBasic: true})))
				return
			}
		}
		httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要管理员登录", http.StatusUnauthorized))
	})
}

func Worker(env string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if env != "local" {
			httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要师傅认证", 401))
			return
		}
		token := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("Authorization")), "Bearer local-worker-")
		id, err := strconv.ParseInt(token, 10, 64)
		if err != nil || id <= 0 {
			httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要师傅认证", 401))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, Principal{OrgID: 1, SubjectID: id, Role: "WORKER", Name: "本地演示师傅"})))
	})
}

type key string

const principalKey key = "principal"

func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}
func Admin(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(username)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="FixPro"`)
			httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要管理员认证", 401))
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, Principal{OrgID: 1, Role: "ADMIN", Name: u})))
	})
}
func Customer(env string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if env == "local" && strings.TrimSpace(r.Header.Get("Authorization")) == "Bearer local-customer-1" {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, Principal{OrgID: 1, SubjectID: 1, Role: "CUSTOMER", Name: "本地演示客户"})))
			return
		}
		httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要客户认证", 401))
	})
}
