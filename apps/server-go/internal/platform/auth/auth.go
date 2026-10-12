package auth

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/fixpro/server/internal/platform/logging"
	"go.uber.org/zap"
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
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
				return
			}
		}
		if allowBasic && env == "local" {
			u, p, ok := r.BasicAuth()
			if ok && subtle.ConstantTimeCompare([]byte(u), []byte(username)) == 1 && subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1 {
				next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), Principal{OrgID: 1, Role: "ADMIN", Name: u, LegacyBasic: true})))
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
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), Principal{OrgID: 1, SubjectID: id, Role: "WORKER", Name: "本地演示师傅"})))
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
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), Principal{OrgID: 1, Role: "ADMIN", Name: u})))
	})
}

type CustomerAuthenticator interface {
	Authenticate(*http.Request) (Principal, error)
}

func Customer(store CustomerAuthenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if store == nil {
			httpx.Failure(w, r, httpx.E("CUSTOMER_SESSION_INVALID", "请先授权手机号登录", 401))
			return
		}
		p, e := store.Authenticate(r)
		if e != nil {
			httpx.Failure(w, r, e)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	id := p.SubjectID
	if p.AdminUserID > 0 {
		id = p.AdminUserID
	}
	logging.Bind(ctx, zap.String("principalType", p.Role), zap.String("principalId", strconv.FormatInt(id, 10)), zap.String("orgId", strconv.FormatInt(p.OrgID, 10)))
	return context.WithValue(ctx, principalKey, p)
}
