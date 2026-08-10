package media

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s: s} }
func (h *Handler) UploadSKU(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, "SKU_IMAGE", true)
}
func (h *Handler) UploadWorker(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("purpose") {
	case "AVATAR":
		h.upload(w, r, "WORKER_AVATAR", true)
	case "CERTIFICATE":
		h.upload(w, r, "WORKER_CERTIFICATE", true)
	default:
		httpx.Failure(w, r, httpx.E("MEDIA_PURPOSE_INVALID", "师傅资料类型不合法", 400))
	}
}
func (h *Handler) UploadFault(w http.ResponseWriter, r *http.Request) {
	h.upload(w, r, "FAULT_EVIDENCE", false)
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request, purpose string, imageOnly bool) {
	p, _ := auth.From(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 52<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_SIZE_EXCEEDED", "上传文件过大或格式错误", 413))
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_NOT_FOUND", "请选择文件", 400))
		return
	}
	_ = f.Close()
	out, err := h.s.Upload(r.Context(), p, fh, purpose, imageOnly)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, out)
}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request)    { h.read(w, r, true) }
func (h *Handler) Protected(w http.ResponseWriter, r *http.Request) { h.read(w, r, false) }
func (h *Handler) read(w http.ResponseWriter, r *http.Request, public bool) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	var p *auth.Principal
	if value, ok := auth.From(r.Context()); ok {
		p = &value
	}
	a, f, err := h.s.Read(r.Context(), id, p, public)
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(a.Size, 10))
	w.Header().Set("Content-Disposition", `inline; filename="media"`)
	if public {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	http.ServeContent(w, r, a.Name, aTime(), f)
}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		httpx.Failure(w, r, err)
		return
	}
	p, _ := auth.From(r.Context())
	if err = h.s.Delete(r.Context(), id, p); err != nil {
		httpx.Failure(w, r, err)
		return
	}
	httpx.Success(w, r, nil)
}
func aTime() (t time.Time) { return time.Unix(0, 0) }
