package app

import (
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/cart"
	"github.com/fixpro/server/internal/catalog"
	"github.com/fixpro/server/internal/media"
	"github.com/fixpro/server/internal/order"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/config"
	"github.com/fixpro/server/internal/platform/httpx"
	"log/slog"
	"net/http"
)

func New(c config.Config, db *sql.DB, log *slog.Logger) (http.Handler, error) {
	if c.MediaDriver != "local" {
		return nil, fmt.Errorf("media driver %q is configured but its production adapter is not included in this slice", c.MediaDriver)
	}
	mux := http.NewServeMux()
	cat := catalog.NewHandler(catalog.New(db))
	crt := cart.NewHandler(cart.New(db))
	ord := order.NewHandler(order.New(db))
	ms, e := media.New(db, c.MediaLocalRoot)
	if e != nil {
		return nil, e
	}
	med := media.NewHandler(ms)
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
	return httpx.Middleware(log, c.CORSAllowedOrigins, mux), nil
}
