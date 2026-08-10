package workforce

import (
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net/http"
	"strconv"
)

type Handler struct{ service *Service }

func NewHandler(s *Service) *Handler { return &Handler{service: s} }
func qInt(r *http.Request, k string) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get(k), 10, 64)
	return n
}
func (h *Handler) Trades(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	x, e := h.service.Trades(r.Context(), p, r.URL.Query().Get("status"))
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, x)
}
func (h *Handler) Skills(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	x, e := h.service.Skills(r.Context(), p, qInt(r, "tradeId"), r.URL.Query().Get("status"), r.URL.Query().Get("keyword"))
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, x)
}
func (h *Handler) CreateTrade(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var x Trade
	if e := httpx.DecodeJSON(w, r, &x); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.CreateTrade(r.Context(), p, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) UpdateTrade(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var x Trade
	id, e := httpx.PathID(r, "id")
	if e == nil {
		e = httpx.DecodeJSON(w, r, &x)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.UpdateTrade(r.Context(), p, id, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) TradeStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	var x struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if e == nil {
		e = httpx.DecodeJSON(w, r, &x)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.SetTradeStatus(r.Context(), p, id, x.Status, x.Version)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) DeleteTrade(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	if e = h.service.DeleteTrade(r.Context(), p, id); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, map[string]bool{"deleted": true})
}
func (h *Handler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var x Skill
	if e := httpx.DecodeJSON(w, r, &x); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.CreateSkill(r.Context(), p, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var x Skill
	id, e := httpx.PathID(r, "id")
	if e == nil {
		e = httpx.DecodeJSON(w, r, &x)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.UpdateSkill(r.Context(), p, id, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) SkillStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	var x struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if e == nil {
		e = httpx.DecodeJSON(w, r, &x)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.SetSkillStatus(r.Context(), p, id, x.Status, x.Version)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	if e = h.service.DeleteSkill(r.Context(), p, id); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, map[string]bool{"deleted": true})
}
func (h *Handler) Workers(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	x, e := h.service.Workers(r.Context(), p, r.URL.Query().Get("status"), r.URL.Query().Get("keyword"))
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, x)
}
func (h *Handler) Worker(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id := qInt(r, "id")
	if id == 0 {
		id, _ = httpx.PathID(r, "id")
	}
	x, e := h.service.worker(r.Context(), p, id)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, x)
}
func (h *Handler) SaveWorker(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id := qInt(r, "id")
	if id == 0 {
		id, _ = httpx.PathID(r, "id")
	}
	var x WorkerWrite
	if e := httpx.DecodeJSON(w, r, &x); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	out, e := h.service.SaveWorker(r.Context(), p, id, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) Activate(w http.ResponseWriter, r *http.Request) { h.status(w, r, "ACTIVE") }
func (h *Handler) Disable(w http.ResponseWriter, r *http.Request)  { h.status(w, r, "DISABLED") }
func (h *Handler) status(w http.ResponseWriter, r *http.Request, status string) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	var x DisableRequest
	if e == nil {
		e = httpx.DecodeJSON(w, r, &x)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	e = h.service.SetStatus(r.Context(), p, id, status, x)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) Candidates(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	x, e := h.service.Candidates(r.Context(), p, qInt(r, "workOrderId"), qInt(r, "tradeId"), qInt(r, "skillId"))
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, x)
}
