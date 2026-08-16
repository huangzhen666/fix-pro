package app

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/address"
	"github.com/fixpro/server/internal/admin"
	"github.com/fixpro/server/internal/authorization"
	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/fulfillment"
	"github.com/fixpro/server/internal/media"
	"github.com/fixpro/server/internal/order"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/fixpro/server/internal/workforce"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func New(c config.Config, db *sql.DB, log *slog.Logger) (http.Handler, error) {
	if c.MediaDriver != "local" {
		return nil, fmt.Errorf("media driver %q is configured but its production adapter is not included in this slice", c.MediaDriver)
	}
	mux := http.NewServeMux()
	cat := catalog.NewHandler(catalog.New(db))
	crt := cart.NewHandler(cart.New(db))
	ms, e := media.New(db, c.MediaLocalRoot)
	if e != nil {
		return nil, e
	}
	med := media.NewHandler(ms)
	ord := order.NewHandler(order.New(db))
	addr := address.NewHandler(address.New(db))
	fulService := fulfillment.New(db, ms)
	ful := fulfillment.NewHandler(fulService)
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		_ = fulService.AutoAcceptDue(context.Background())
		for range ticker.C {
			if err := fulService.AutoAcceptDue(context.Background()); err != nil {
				log.Error("auto acceptance", "error", err)
			}
		}
	}()
	workerSessions := auth.NewWorkerSessionStore(db, c.WorkerDevTokenEnabled)
	workService := workforce.New(db)
	work := workforce.NewHandler(workService)
	work.SetWorkerSessionStore(workerSessions)
	workerAuth := workforce.NewAuthHandler(workService, workerSessions)
	authorizer := authorization.New(db)
	sessions := auth.NewSessionStore(db, c.AdminCookieSecure)
	adminAPI := admin.NewHandler(db, sessions, authorizer)
	mux.HandleFunc("GET /actuator/health", func(w http.ResponseWriter, r *http.Request) {
		if e := db.PingContext(r.Context()); e != nil {
			httpx.Failure(w, r, e)
			return
		}
		httpx.Success(w, r, map[string]string{"status": "UP"})
	})
	mux.HandleFunc("GET /api/v1/public/ping", func(w http.ResponseWriter, r *http.Request) {
		httpx.Success(w, r, map[string]string{"message": "pong"})
	})
	mux.HandleFunc("GET /api/v1/public/media/{id}", med.Public)
	mux.HandleFunc("GET /api/v1/catalog/services", cat.PublicList)
	mux.HandleFunc("GET /api/v1/catalog/services/{id}", cat.PublicDetail)
	mux.HandleFunc("GET /api/v1/catalog/categories", cat.PublicGroups)
	admin := func(h http.HandlerFunc) http.Handler {
		protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.From(r.Context())
			if !ok {
				httpx.Failure(w, r, httpx.E("UNAUTHORIZED", "需要管理员登录", http.StatusUnauthorized))
				return
			}
			if p.SessionAuthenticated && p.MustChangePassword && r.URL.Path != "/api/v1/admin/auth/me" && r.URL.Path != "/api/v1/admin/auth/password" && r.URL.Path != "/api/v1/admin/auth/logout" {
				httpx.Failure(w, r, httpx.E("PASSWORD_CHANGE_REQUIRED", "首次登录请先修改密码", http.StatusLocked))
				return
			}
			if !p.LegacyBasic {
				code := adminPermission(r)
				allowed, err := authorizer.Check(r.Context(), authorization.Subject{UserID: p.AdminUserID, OrgID: p.OrgID, PlatformSuperAdmin: p.PlatformSuperAdmin}, code)
				if err != nil {
					httpx.Failure(w, r, err)
					return
				}
				if !allowed {
					httpx.Failure(w, r, httpx.E("FORBIDDEN", "没有执行该操作的权限", http.StatusForbidden))
					return
				}
			}
			h.ServeHTTP(w, r)
		})
		return auth.AdminSessionOrBasic(c.Env, sessions, c.AdminUsername, c.AdminPassword, c.AdminBasicCompat, protected)
	}
	// Authentication endpoints are intentionally outside the business permission boundary.
	mux.HandleFunc("POST /api/v1/admin/auth/login", adminAPI.Login)
	mux.Handle("POST /api/v1/admin/auth/logout", auth.AdminSessionOrBasic(c.Env, sessions, c.AdminUsername, c.AdminPassword, c.AdminBasicCompat, http.HandlerFunc(adminAPI.Logout)))
	mux.Handle("GET /api/v1/admin/auth/me", auth.AdminSessionOrBasic(c.Env, sessions, c.AdminUsername, c.AdminPassword, c.AdminBasicCompat, http.HandlerFunc(adminAPI.Me)))
	mux.Handle("POST /api/v1/admin/auth/password", auth.AdminSessionOrBasic(c.Env, sessions, c.AdminUsername, c.AdminPassword, false, http.HandlerFunc(adminAPI.ChangePassword)))
	mux.Handle("GET /api/v1/admin/permissions", admin(adminAPI.Permissions))
	mux.Handle("GET /api/v1/admin/roles", admin(adminAPI.Roles))
	mux.Handle("GET /api/v1/admin/roles/{id}", admin(adminAPI.RoleDetail))
	mux.Handle("POST /api/v1/admin/roles", admin(adminAPI.CreateRole))
	mux.Handle("PUT /api/v1/admin/roles/{id}", admin(adminAPI.UpdateRole))
	mux.Handle("DELETE /api/v1/admin/roles/{id}", admin(adminAPI.DeleteRole))
	mux.Handle("GET /api/v1/admin/users", admin(adminAPI.Users))
	mux.Handle("POST /api/v1/admin/users", admin(adminAPI.CreateUser))
	mux.Handle("PUT /api/v1/admin/users/{id}", admin(adminAPI.UpdateUser))
	mux.Handle("POST /api/v1/admin/users/{id}/status", admin(adminAPI.SetUserStatus))
	mux.Handle("POST /api/v1/admin/users/{id}/reset-password", admin(adminAPI.ResetPassword))
	mux.Handle("GET /api/v1/admin/users/{id}/permissions", admin(adminAPI.EffectivePermissions))
	mux.Handle("GET /api/v1/admin/audit-logs", admin(adminAPI.AuditLogs))
	mux.Handle("POST /api/v1/admin/media/images", admin(med.UploadSKU))
	mux.Handle("POST /api/v1/admin/media/worker", admin(med.UploadWorker))
	mux.Handle("GET /api/v1/admin/media/{id}/content", admin(med.Protected))
	mux.Handle("DELETE /api/v1/admin/media/{id}", admin(med.Delete))
	mux.Handle("GET /api/v1/admin/catalog/categories", admin(cat.Categories))
	mux.Handle("POST /api/v1/admin/catalog/categories", admin(cat.CreateCategory))
	mux.Handle("PUT /api/v1/admin/catalog/categories/{id}", admin(cat.UpdateCategory))
	mux.Handle("POST /api/v1/admin/catalog/categories/{id}/status", admin(cat.CategoryStatus))
	mux.Handle("GET /api/v1/admin/catalog/skus", admin(cat.AdminList))
	mux.Handle("GET /api/v1/admin/catalog/skus/{id}", admin(cat.AdminDetail))
	mux.Handle("POST /api/v1/admin/catalog/skus", admin(cat.Create))
	mux.Handle("PUT /api/v1/admin/catalog/skus/{id}", admin(cat.Update))
	mux.Handle("POST /api/v1/admin/catalog/skus/{id}/publish", admin(cat.Publish))
	mux.Handle("POST /api/v1/admin/catalog/skus/{id}/off-shelf", admin(cat.OffShelf))
	mux.Handle("GET /api/v1/admin/orders", admin(ord.List))
	mux.Handle("GET /api/v1/admin/orders/{id}", admin(ord.Detail))
	mux.Handle("POST /api/v1/admin/orders/{id}/confirm", admin(ful.ConfirmOrder))
	mux.Handle("GET /api/v1/admin/worker-trades", admin(work.Trades))
	mux.Handle("POST /api/v1/admin/worker-trades", admin(work.CreateTrade))
	mux.Handle("PUT /api/v1/admin/worker-trades/{id}", admin(work.UpdateTrade))
	mux.Handle("POST /api/v1/admin/worker-trades/{id}/status", admin(work.TradeStatus))
	mux.Handle("DELETE /api/v1/admin/worker-trades/{id}", admin(work.DeleteTrade))
	mux.Handle("GET /api/v1/admin/worker-skills", admin(work.Skills))
	mux.Handle("POST /api/v1/admin/worker-skills", admin(work.CreateSkill))
	mux.Handle("PUT /api/v1/admin/worker-skills/{id}", admin(work.UpdateSkill))
	mux.Handle("POST /api/v1/admin/worker-skills/{id}/status", admin(work.SkillStatus))
	mux.Handle("DELETE /api/v1/admin/worker-skills/{id}", admin(work.DeleteSkill))
	mux.Handle("GET /api/v1/admin/workers", admin(work.Workers))
	mux.Handle("POST /api/v1/admin/workers", admin(work.SaveWorker))
	mux.Handle("GET /api/v1/admin/workers/{id}", admin(work.Worker))
	mux.Handle("PUT /api/v1/admin/workers/{id}", admin(work.SaveWorker))
	mux.Handle("POST /api/v1/admin/workers/{id}/activate", admin(work.Activate))
	mux.Handle("POST /api/v1/admin/workers/{id}/disable", admin(work.Disable))
	mux.Handle("POST /api/v1/admin/workers/{id}/reset-password", admin(work.ResetPassword))
	mux.Handle("GET /api/v1/admin/workers/candidates", admin(work.Candidates))
	mux.Handle("GET /api/v1/admin/work-orders", admin(ful.WorkOrders))
	mux.Handle("GET /api/v1/admin/work-orders/{id}", admin(ful.AdminWorkOrderDetail))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/assign", admin(ful.Assign))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/reassign", admin(ful.Reassign))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/reschedule", admin(ful.Reschedule))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/completion-review", admin(ful.ReviewCompletion))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/internal-review", admin(ful.InternalReview))
	mux.Handle("POST /api/v1/admin/work-orders/{id}/customer-service-confirmation", admin(ful.CustomerServiceConfirmation))
	mux.Handle("GET /api/v1/admin/work-orders/{id}/timeline", admin(ful.WorkOrderTimeline))
	mux.HandleFunc("POST /api/v1/worker/auth/login", workerAuth.Login)
	workerAuthEndpoint := func(h http.HandlerFunc) http.Handler { return auth.WorkerSession(workerSessions, h) }
	mux.Handle("GET /api/v1/worker/auth/me", workerAuthEndpoint(workerAuth.Me))
	mux.Handle("POST /api/v1/worker/auth/logout", workerAuthEndpoint(workerAuth.Logout))
	mux.Handle("POST /api/v1/worker/auth/password", workerAuthEndpoint(workerAuth.ChangePassword))
	worker := func(h http.HandlerFunc) http.Handler {
		protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := auth.From(r.Context())
			if !ok || p.Role != "WORKER" {
				httpx.Failure(w, r, httpx.E("WORKER_SESSION_INVALID", "需要师傅登录", http.StatusUnauthorized))
				return
			}
			if p.MustChangePassword {
				httpx.Failure(w, r, httpx.E("WORKER_PASSWORD_CHANGE_REQUIRED", "首次登录请先修改密码", http.StatusLocked))
				return
			}
			h.ServeHTTP(w, r)
		})
		return auth.WorkerSession(workerSessions, protected)
	}
	mux.Handle("GET /api/v1/worker/work-orders", worker(ful.WorkerList))
	mux.Handle("GET /api/v1/worker/work-orders/{id}", worker(ful.WorkerWorkOrder))
	mux.Handle("GET /api/v1/worker/media/{id}/content", worker(med.Protected))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/{command}", worker(ful.WorkerCommand))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/reschedule", worker(ful.WorkerReschedule))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/return", worker(ful.WorkerReturn))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/media/images", worker(ful.UploadEvidence))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/evidence", worker(ful.BindEvidence))
	mux.Handle("POST /api/v1/worker/work-orders/{id}/submit-completion", worker(ful.SubmitCompletion))
	customer := func(h http.HandlerFunc) http.Handler { return auth.Customer(c.Env, h) }
	mux.Handle("POST /api/v1/mini/media/fault", customer(med.UploadFault))
	mux.Handle("GET /api/v1/mini/media/{id}/content", customer(med.Protected))
	mux.Handle("DELETE /api/v1/mini/media/{id}", customer(med.Delete))
	mux.Handle("GET /api/v1/mini/cart", customer(crt.Get))
	mux.Handle("POST /api/v1/mini/cart/items", customer(crt.Add))
	mux.Handle("PATCH /api/v1/mini/cart/items/{id}", customer(crt.Quantity))
	mux.Handle("PUT /api/v1/mini/cart/items/{id}/fault", customer(crt.Fault))
	mux.Handle("DELETE /api/v1/mini/cart/items/{id}", customer(crt.Delete))
	mux.Handle("POST /api/v1/mini/orders", customer(ord.Create))
	mux.Handle("GET /api/v1/mini/addresses", customer(addr.List))
	mux.Handle("POST /api/v1/mini/addresses", customer(addr.Create))
	mux.Handle("PUT /api/v1/mini/addresses/{id}", customer(addr.Update))
	mux.Handle("POST /api/v1/mini/addresses/{id}/default", customer(addr.SetDefault))
	mux.Handle("DELETE /api/v1/mini/addresses/{id}", customer(addr.Delete))
	mux.Handle("GET /api/v1/mini/orders", customer(ful.CustomerOrders))
	mux.Handle("GET /api/v1/mini/orders/{id}", customer(ful.CustomerOrder))
	mux.Handle("GET /api/v1/mini/work-orders/{id}", customer(ful.CustomerWorkOrder))
	mux.Handle("POST /api/v1/mini/work-orders/{id}/acceptance", customer(ful.CustomerAcceptance))
	mux.Handle("POST /api/v1/mini/work-orders/{id}/rating", customer(ful.SubmitRating))
	mux.Handle("GET /api/v1/mini/work-orders/{id}/timeline", customer(ful.WorkOrderTimeline))
	return httpx.Middleware(log, c.CORSAllowedOrigins, mux), nil
}

func adminPermission(r *http.Request) string {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/api/v1/admin/auth/"):
		return ""
	case path == "/api/v1/admin/permissions":
		return "admin.role.view"
	case strings.HasPrefix(path, "/api/v1/admin/roles"):
		if r.Method == http.MethodGet {
			return "admin.role.view"
		}
		if r.Method == http.MethodPost {
			return "admin.role.create"
		}
		if r.Method == http.MethodDelete {
			return "admin.role.delete"
		}
		return "admin.role.update"
	case strings.HasPrefix(path, "/api/v1/admin/users"):
		if strings.HasSuffix(path, "/reset-password") {
			return "admin.user.reset_password"
		}
		if strings.HasSuffix(path, "/status") {
			return "admin.user.disable"
		}
		if r.Method == http.MethodGet {
			return "admin.user.view"
		}
		if r.Method == http.MethodPost {
			return "admin.user.create"
		}
		if r.Method == http.MethodDelete {
			return "admin.user.disable"
		}
		return "admin.user.update"
	case strings.HasPrefix(path, "/api/v1/admin/catalog/categories"):
		if r.Method == http.MethodGet {
			return "catalog.category.view"
		}
		return "catalog.category.manage"
	case strings.HasPrefix(path, "/api/v1/admin/catalog/skus"):
		if strings.HasSuffix(path, "/publish") || strings.HasSuffix(path, "/off-shelf") {
			return "catalog.sku.publish"
		}
		if r.Method == http.MethodGet {
			return "catalog.sku.view"
		}
		if r.Method == http.MethodPost {
			return "catalog.sku.create"
		}
		return "catalog.sku.update"
	case strings.HasPrefix(path, "/api/v1/admin/orders"):
		if strings.HasSuffix(path, "/confirm") {
			return "order.confirm"
		}
		return "order.view"
	case strings.HasPrefix(path, "/api/v1/admin/work-orders"):
		switch {
		case strings.HasSuffix(path, "/assign"), strings.HasSuffix(path, "/reassign"):
			return "fulfillment.dispatch"
		case strings.HasSuffix(path, "/reschedule"):
			return "fulfillment.reschedule"
		case strings.HasSuffix(path, "/completion-review"):
			return "fulfillment.qa_review"
		case strings.HasSuffix(path, "/internal-review"):
			return "fulfillment.director_review"
		case strings.HasSuffix(path, "/customer-service-confirmation"):
			return "fulfillment.customer_service"
		default:
			return "fulfillment.view"
		}
	case strings.HasPrefix(path, "/api/v1/admin/workers"):
		if strings.HasSuffix(path, "/reset-password") {
			return "worker.reset_password"
		}
		if r.Method == http.MethodGet {
			return "worker.view"
		}
		return "worker.manage"
	case strings.HasPrefix(path, "/api/v1/admin/worker-trades"), strings.HasPrefix(path, "/api/v1/admin/worker-skills"):
		if r.Method == http.MethodGet {
			return "worker.skill.view"
		}
		return "worker.skill.manage"
	case strings.HasPrefix(path, "/api/v1/admin/media"):
		return "media.manage"
	case strings.HasPrefix(path, "/api/v1/admin/audit-logs"):
		return "audit.view"
	default:
		// New admin routes must be explicitly added to the permission matrix.
		return "__route_not_registered__"
	}
}
