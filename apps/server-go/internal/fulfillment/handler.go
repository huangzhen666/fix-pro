package fulfillment

import (
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) ConfirmOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req ConfirmRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.ConfirmOrder(r.Context(), p, r.Header.Get("Idempotency-Key"), id, req)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}

func (h *Handler) Workers(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	out, err := h.service.Workers(r.Context(), p.OrgID, r.URL.Query().Get("status"))
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) CreateWorker(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		Mobile      string `json:"mobile"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.CreateWorker(r.Context(), p, req.Username, req.DisplayName, req.Mobile)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) WorkerStatus(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	err = h.service.SetWorkerStatus(r.Context(), p, id, strings.TrimSpace(req.Status))
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) WorkOrders(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	worker, _ := strconv.ParseInt(r.URL.Query().Get("workerId"), 10, 64)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	out, err := h.service.WorkOrders(r.Context(), p.OrgID, r.URL.Query().Get("status"), worker, r.URL.Query().Get("keyword"), r.URL.Query().Get("completionOutcome"), page, size)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) AdminWorkOrderDetail(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.AdminWorkOrderDetail(r.Context(), p, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) Assign(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req AssignRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	err = h.service.Assign(r.Context(), p, id, req)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) Reassign(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req ReassignRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	err = h.service.Reassign(r.Context(), p, id, req)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) Reschedule(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req RescheduleRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	err = h.service.Reschedule(r.Context(), p, id, req)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) WorkerReschedule(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req WorkerRescheduleRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.WorkerReschedule(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) WorkerReturn(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req struct {
		Version int    `json:"version"`
		Reason  string `json:"reason"`
	}
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.WorkerReturn(r.Context(), p, id, req.Version, req.Reason); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) WorkerList(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	out, err := h.service.WorkOrders(r.Context(), p.OrgID, r.URL.Query().Get("status"), p.SubjectID, "", "", 1, 100)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) WorkerWorkOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.WorkerWorkOrder(r.Context(), p, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) WorkerCommand(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req RejectRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	command := r.PathValue("command")
	err = h.service.WorkerCommand(r.Context(), p, id, command, req, r.Header.Get("Idempotency-Key"))
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) UploadEvidence(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 52<<20)
	if err = r.ParseMultipartForm(1 << 20); err != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_SIZE_EXCEEDED", "图片或视频超过大小限制", 413))
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_NOT_FOUND", "请选择图片或视频", 400))
		return
	}
	_ = f.Close()
	out, err := h.service.UploadEvidence(r.Context(), p, id, fh)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}

func (h *Handler) BindEvidence(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req EvidenceRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.BindEvidence(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) SubmitCompletion(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req CompletionRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.SubmitCompletion(r.Context(), p, id, req, r.Header.Get("Idempotency-Key")); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) ReviewCompletion(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req ReviewRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.ReviewCompletion(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) InternalReview(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	level := r.URL.Query().Get("level")
	var req ReviewLevelRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.InternalReview(r.Context(), p, id, level, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) CustomerOrders(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	out, err := h.service.CustomerOrders(r.Context(), p, page, size)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) CustomerOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.CustomerOrder(r.Context(), p, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) CustomerWorkOrder(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.CustomerWorkOrder(r.Context(), p, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) CustomerAcceptance(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req AcceptanceRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.CustomerAcceptance(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}

func (h *Handler) SubmitRating(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req RatingRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.SubmitRating(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"created": true})
}
func (h *Handler) CustomerServiceConfirmation(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var req ServiceConfirmationRequest
	if err = httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	if err = h.service.CustomerServiceConfirmation(r.Context(), p, id, req); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]bool{"updated": true})
}
func (h *Handler) WorkOrderTimeline(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	out, err := h.service.WorkOrderTimeline(r.Context(), p, id)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, map[string]any{"items": out})
}
