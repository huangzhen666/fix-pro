package media

import (
	"bytes"
	"context"
	"database/sql"
	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/database"
	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/fixpro/server/internal/platform/logging"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func ValidateAvatar(fh *multipart.FileHeader) error {
	if fh.Size <= 0 || fh.Size > 5<<20 {
		return httpx.E("MEDIA_SIZE_EXCEEDED", "头像不能超过 5 MB", 413)
	}
	f, e := fh.Open()
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if e != nil {
		return e
	}
	if len(b) > 5<<20 {
		return httpx.E("MEDIA_SIZE_EXCEEDED", "头像不能超过 5 MB", 413)
	}
	ct, kind, _ := detect(b)
	if kind != "IMAGE" || ct == "" {
		return httpx.E("MEDIA_TYPE_NOT_SUPPORTED", "头像仅支持 JPEG、PNG、WebP", 400)
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return httpx.E("MEDIA_TYPE_NOT_SUPPORTED", "头像图片无效或尺寸过大", 400)
	}
	if _, _, e = image.Decode(bytes.NewReader(b)); e != nil {
		return httpx.E("MEDIA_TYPE_NOT_SUPPORTED", "头像图片无法解码", 400)
	}
	return nil
}
func (h *Handler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, (5<<20)+(64<<10))
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_SIZE_EXCEEDED", "头像过大或格式错误", 413))
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, fh, e := r.FormFile("file")
	if e != nil {
		httpx.Failure(w, r, httpx.E("MEDIA_NOT_FOUND", "请选择头像", 400))
		return
	}
	f.Close()
	if e = ValidateAvatar(fh); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	p, _ := auth.From(r.Context())
	out, e := h.s.Upload(r.Context(), p, fh, "CUSTOMER_AVATAR", true)
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	httpx.Success(w, r, map[string]any{"mediaId": out.ID, "size": out.Size})
}
func (h *Handler) CustomerAvatar(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	id, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var mediaID, owner int64
	e = h.s.db.QueryRowContext(r.Context(), `SELECT m.id,c.id FROM customer c JOIN media_asset m ON m.id=c.avatar_media_id AND m.org_id=c.org_id AND m.owner_type='CUSTOMER' AND m.owner_id=c.id AND m.purpose='CUSTOMER_AVATAR' WHERE c.org_id=$1 AND c.id=$2 AND c.source_channel='WECHAT_MINI'`, p.OrgID, id).Scan(&mediaID, &owner)
	if e == sql.ErrNoRows {
		e = httpx.E("MEDIA_NOT_FOUND", "头像不存在", 404)
	}
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	r.SetPathValue("id", strconv.FormatInt(mediaID, 10))
	h.Protected(w, r)
}

// Media row locks serialize garbage collection with profile references.
func (s *Service) CollectAvatars(ctx context.Context) error {
	tx, e := database.BeginTx(ctx, s.db)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rows, e := tx.QueryContext(ctx, `SELECT id,object_key FROM media_asset m WHERE purpose='CUSTOMER_AVATAR' AND status='READY' AND created_at<now()-interval '24 hours' AND NOT EXISTS(SELECT 1 FROM customer c WHERE c.avatar_media_id=m.id) ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if e != nil {
		return e
	}
	type object struct {
		id  int64
		key string
	}
	items := []object{}
	for rows.Next() {
		var x object
		if e = rows.Scan(&x.id, &x.key); e != nil {
			rows.Close()
			return e
		}
		items = append(items, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, x := range items {
		_, e = tx.ExecContext(ctx, `UPDATE media_asset SET status='VOID' WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM customer WHERE avatar_media_id=$1)`, x.id)
		if e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	// VOID rows retain object keys so a failed filesystem removal can be retried.
	rows, e = s.db.QueryContext(ctx, `SELECT object_key FROM media_asset WHERE purpose='CUSTOMER_AVATAR' AND status='VOID' ORDER BY id LIMIT 1000`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if e = rows.Scan(&key); e != nil {
			return e
		}
		path := filepath.Join(s.root, filepath.FromSlash(key))
		if !strings.HasPrefix(filepath.Clean(path), s.root+string(os.PathSeparator)) {
			return httpx.E("MEDIA_ACCESS_DENIED", "非法媒体路径", 403)
		}
		if e = os.Remove(path); e != nil && !os.IsNotExist(e) {
			logging.Count(ctx, false)
			return e
		} else if e == nil {
			logging.Count(ctx, true)
		}
	}
	return rows.Err()
}
