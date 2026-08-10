package app

import (
	"context"
	"database/sql"
	"fmt"
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
	work := workforce.NewHandler(workforce.New(db))
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
	admin := func(h http.HandlerFunc) http.Handler { return auth.Admin(c.AdminUsername, c.AdminPassword, h) }
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
	worker := func(h http.HandlerFunc) http.Handler { return auth.Worker(c.Env, h) }
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
	mux.Handle("GET /api/v1/mini/orders", customer(ful.CustomerOrders))
	mux.Handle("GET /api/v1/mini/orders/{id}", customer(ful.CustomerOrder))
	mux.Handle("GET /api/v1/mini/work-orders/{id}", customer(ful.CustomerWorkOrder))
	mux.Handle("POST /api/v1/mini/work-orders/{id}/acceptance", customer(ful.CustomerAcceptance))
	mux.Handle("POST /api/v1/mini/work-orders/{id}/rating", customer(ful.SubmitRating))
	mux.Handle("GET /api/v1/mini/work-orders/{id}/timeline", customer(ful.WorkOrderTimeline))
	return httpx.Middleware(log, c.CORSAllowedOrigins, mux), nil
}
