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
	OrgID, SubjectID int64
	Role, Name       string
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
