package media

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
)

type Service struct {
	db   *sql.DB
	root string
}
type Uploaded struct {
	ID          string `json:"id"`
	MediaType   string `json:"mediaType"`
	ContentType string `json:"contentType"`
	Size        int64  `json:"size"`
}
type asset struct {
	ID, OrgID, OwnerID                                            int64
	OwnerType, Purpose, MediaType, Name, Key, ContentType, Status string
	Size                                                          int64
}

func New(db *sql.DB, root string) (*Service, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0750); err != nil {
		return nil, err
	}
	return &Service{db: db, root: root}, nil
}

func (s *Service) Upload(ctx context.Context, p auth.Principal, fh *multipart.FileHeader, purpose string, imageOnly bool) (Uploaded, error) {
	if fh == nil {
		return Uploaded{}, httpx.E("MEDIA_NOT_FOUND", "请选择文件", 400)
	}
	max := int64(50 << 20)
	if imageOnly {
		max = 10 << 20
	}
	if fh.Size <= 0 || fh.Size > max {
		return Uploaded{}, httpx.E("MEDIA_SIZE_EXCEEDED", "文件超过大小限制", 413)
	}
	f, err := fh.Open()
	if err != nil {
		return Uploaded{}, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	head, err := br.Peek(min(16, int(fh.Size)))
	if err != nil && err != io.EOF {
		return Uploaded{}, err
	}
	ct, kind, ext := detect(head)
	if ct == "" || (imageOnly && kind != "IMAGE") {
		return Uploaded{}, httpx.E("MEDIA_TYPE_NOT_SUPPORTED", "文件内容或类型不支持", 400)
	}
	if kind == "IMAGE" && fh.Size > 10<<20 || kind == "VIDEO" && fh.Size > 50<<20 {
		return Uploaded{}, httpx.E("MEDIA_SIZE_EXCEEDED", "文件超过大小限制", 413)
	}
	key := strings.ToLower(purpose) + "/" + randomName() + ext
	target := filepath.Clean(filepath.Join(s.root, filepath.FromSlash(key)))
	if !strings.HasPrefix(target, s.root+string(os.PathSeparator)) {
		return Uploaded{}, httpx.E("MEDIA_ACCESS_DENIED", "非法媒体路径", 403)
	}
	if err = os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return Uploaded{}, err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return Uploaded{}, err
	}
	written, copyErr := io.Copy(out, br)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written != fh.Size {
		_ = os.Remove(target)
		if copyErr != nil {
			return Uploaded{}, copyErr
		}
		if closeErr != nil {
			return Uploaded{}, closeErr
		}
		return Uploaded{}, errors.New("uploaded size mismatch")
	}
	ownerType, ownerID := "ADMIN", int64(0)
	if p.Role == "CUSTOMER" {
		ownerType, ownerID = "CUSTOMER", p.SubjectID
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `INSERT INTO media_asset(org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'READY') RETURNING id`, p.OrgID, ownerType, ownerID, purpose, kind, safeName(fh.Filename), key, ct, written).Scan(&id)
	if err != nil {
		_ = os.Remove(target)
		return Uploaded{}, err
	}
	return Uploaded{ID: fmt.Sprint(id), MediaType: kind, ContentType: ct, Size: written}, nil
}

func (s *Service) Read(ctx context.Context, id int64, p *auth.Principal, public bool) (asset, *os.File, error) {
	a, err := s.get(ctx, id)
	if err != nil {
		return asset{}, nil, err
	}
	if a.Status != "READY" {
		return asset{}, nil, httpx.E("MEDIA_NOT_READY", "媒体不可用", 409)
	}
	if public {
		var n int
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_sku s LEFT JOIN service_sku_media sm ON sm.sku_id=s.id AND sm.org_id=s.org_id WHERE s.org_id=$1 AND s.status='PUBLISHED' AND (s.cover_media_id=$2 OR sm.media_id=$3)`, a.OrgID, id, id).Scan(&n)
		if err != nil {
			return asset{}, nil, err
		}
		if n == 0 {
			return asset{}, nil, httpx.E("MEDIA_ACCESS_DENIED", "媒体未公开", 403)
		}
	} else if p != nil && p.Role == "CUSTOMER" && (a.OrgID != p.OrgID || a.OwnerType != "CUSTOMER" || a.OwnerID != p.SubjectID) {
		return asset{}, nil, httpx.E("MEDIA_ACCESS_DENIED", "无权访问该媒体", 403)
	}
	path := filepath.Clean(filepath.Join(s.root, filepath.FromSlash(a.Key)))
	if !strings.HasPrefix(path, s.root+string(os.PathSeparator)) {
		return asset{}, nil, httpx.E("MEDIA_ACCESS_DENIED", "非法媒体路径", 403)
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return asset{}, nil, httpx.E("MEDIA_NOT_FOUND", "媒体文件不存在", 404)
	}
	return a, f, err
}
func (s *Service) Delete(ctx context.Context, id int64, p auth.Principal) error {
	a, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	if p.Role == "CUSTOMER" && (a.OwnerType != "CUSTOMER" || a.OwnerID != p.SubjectID) {
		return httpx.E("MEDIA_ACCESS_DENIED", "无权删除该媒体", 403)
	}
	var refs int
	if err = s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM service_sku_media WHERE media_id=$1) + (SELECT COUNT(*) FROM shopping_cart_item_media WHERE media_id=$2) + (SELECT COUNT(*) FROM order_item_media WHERE media_id=$3)`, id, id, id).Scan(&refs); err != nil {
		return err
	}
	if refs > 0 {
		return httpx.E("MEDIA_ACCESS_DENIED", "媒体已被业务引用", 403)
	}
	if _, err = s.db.ExecContext(ctx, "UPDATE media_asset SET status='VOID' WHERE id=$1", id); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.root, filepath.FromSlash(a.Key)))
	return nil
}
func (s *Service) get(ctx context.Context, id int64) (asset, error) {
	var a asset
	err := s.db.QueryRowContext(ctx, `SELECT id,org_id,owner_type,owner_id,purpose,media_type,original_name,object_key,content_type,size_bytes,status FROM media_asset WHERE id=$1`, id).Scan(&a.ID, &a.OrgID, &a.OwnerType, &a.OwnerID, &a.Purpose, &a.MediaType, &a.Name, &a.Key, &a.ContentType, &a.Size, &a.Status)
	if err == sql.ErrNoRows {
		return asset{}, httpx.E("MEDIA_NOT_FOUND", "媒体不存在", 404)
	}
	return a, err
}
func detect(h []byte) (string, string, string) {
	if len(h) >= 3 && h[0] == 0xff && h[1] == 0xd8 && h[2] == 0xff {
		return "image/jpeg", "IMAGE", ".jpg"
	}
	if len(h) >= 8 && string(h[1:4]) == "PNG" && h[0] == 0x89 {
		return "image/png", "IMAGE", ".png"
	}
	if len(h) >= 12 && string(h[:4]) == "RIFF" && string(h[8:12]) == "WEBP" {
		return "image/webp", "IMAGE", ".webp"
	}
	if len(h) >= 8 && string(h[4:8]) == "ftyp" {
		return "video/mp4", "VIDEO", ".mp4"
	}
	return "", "", ""
}
func randomName() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func safeName(v string) string {
	return strings.NewReplacer("\r", "_", "\n", "_", "\"", "_", "\\", "_").Replace(filepath.Base(v))
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
