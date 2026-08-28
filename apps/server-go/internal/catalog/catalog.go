package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/fixpro/server/internal/platform/auth"
	"github.com/fixpro/server/internal/platform/httpx"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct{ db *sql.DB }

func New(db *sql.DB) *Service { return &Service{db: db} }

type Category struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Name      string  `json:"name"`
	SortOrder int     `json:"sortOrder"`
	Status    string  `json:"status"`
	SKUCount  int     `json:"skuCount"`
}
type CategoryWrite struct {
	ParentID  *string `json:"parentId"`
	Name      string  `json:"name"`
	SortOrder int     `json:"sortOrder"`
}
type CategoryGroup struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Services []PublishedSKU `json:"services"`
}
type SKUWrite struct {
	CategoryID, SKUCode, Name, Description, ServiceScope, Exclusions, WarrantyDescription, PriceMode, Unit, CoverMediaID string
	BasePrice                                                                                                            int64
	GalleryMediaIDs                                                                                                      []string
	Version                                                                                                              int
	RequiredSkillIDs                                                                                                     []string
}

func (s *SKUWrite) UnmarshalJSON(b []byte) error {
	type raw struct {
		CategoryID          string   `json:"categoryId"`
		SKUCode             string   `json:"skuCode"`
		Name                string   `json:"name"`
		Description         string   `json:"description"`
		ServiceScope        string   `json:"serviceScope"`
		Exclusions          string   `json:"exclusions"`
		WarrantyDescription string   `json:"warrantyDescription"`
		PriceMode           string   `json:"priceMode"`
		BasePrice           int64    `json:"basePrice"`
		Unit                string   `json:"unit"`
		CoverMediaID        string   `json:"coverMediaId"`
		GalleryMediaIDs     []string `json:"galleryMediaIds"`
		Version             int      `json:"version"`
		RequiredSkillIDs    []string `json:"requiredSkillIds"`
	}
	var v raw
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = SKUWrite{CategoryID: v.CategoryID, SKUCode: v.SKUCode, Name: v.Name, Description: v.Description, ServiceScope: v.ServiceScope, Exclusions: v.Exclusions, WarrantyDescription: v.WarrantyDescription, PriceMode: v.PriceMode, Unit: v.Unit, CoverMediaID: v.CoverMediaID, BasePrice: v.BasePrice, GalleryMediaIDs: v.GalleryMediaIDs, Version: v.Version, RequiredSkillIDs: v.RequiredSkillIDs}
	return nil
}

type SKUDetail struct {
	ID                  string    `json:"id"`
	CategoryID          string    `json:"categoryId"`
	SKUCode             string    `json:"skuCode"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	ServiceScope        string    `json:"serviceScope"`
	Exclusions          string    `json:"exclusions"`
	WarrantyDescription string    `json:"warrantyDescription"`
	PriceMode           string    `json:"priceMode"`
	BasePrice           int64     `json:"basePrice"`
	Unit                string    `json:"unit"`
	CoverMediaID        *string   `json:"coverMediaId"`
	GalleryMediaIDs     []string  `json:"galleryMediaIds"`
	Status              string    `json:"status"`
	Version             int       `json:"version"`
	PublishedVersion    *int      `json:"publishedVersion"`
	RequiredSkillIDs    []string  `json:"requiredSkillIds"`
	CreatedBy           string    `json:"createdBy"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedBy           string    `json:"updatedBy"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
type SKUAdmin struct {
	ID               string    `json:"id"`
	SKUCode          string    `json:"skuCode"`
	Name             string    `json:"name"`
	CategoryName     string    `json:"categoryName"`
	BasePrice        int64     `json:"basePrice"`
	Unit             string    `json:"unit"`
	Status           string    `json:"status"`
	PublishedVersion *int      `json:"publishedVersion"`
	UpdatedAt        time.Time `json:"updatedAt"`
	CreatedAt        time.Time `json:"createdAt"`
	CreatedBy        string    `json:"createdBy"`
	UpdatedBy        string    `json:"updatedBy"`
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}
type PublishedSKU struct {
	ID                  string   `json:"id"`
	SKUCode             string   `json:"skuCode"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	ServiceScope        string   `json:"serviceScope"`
	Exclusions          string   `json:"exclusions"`
	WarrantyDescription string   `json:"warrantyDescription"`
	PriceMode           string   `json:"priceMode"`
	Price               int64    `json:"price"`
	Unit                string   `json:"unit"`
	CoverImageURL       string   `json:"coverImageUrl"`
	GalleryImageURLs    []string `json:"galleryImageUrls"`
	PublishedVersion    int      `json:"publishedVersion"`
}
type PublishResult struct {
	SKUID            string    `json:"skuId"`
	Status           string    `json:"status"`
	PublishedVersion int       `json:"publishedVersion"`
	PublishedAt      time.Time `json:"publishedAt"`
}

func samePublishedSKU(a, b PublishedSKU) bool {
	a.PublishedVersion = 0
	b.PublishedVersion = 0
	return reflect.DeepEqual(a, b)
}

func id(v string) (int64, error) {
	var n int64
	_, e := fmt.Sscan(v, &n)
	if e != nil || n <= 0 {
		return 0, httpx.E("VALIDATION_ERROR", "ID 格式错误", 400)
	}
	return n, nil
}
func nullable(n sql.NullInt64) *string {
	if !n.Valid {
		return nil
	}
	v := fmt.Sprint(n.Int64)
	return &v
}
func (s *Service) Categories(ctx context.Context, all bool) ([]Category, error) {
	q := `SELECT c.id,c.parent_id,c.name,c.sort_order,c.status,(SELECT COUNT(*) FROM service_sku x WHERE x.org_id=c.org_id AND x.category_id=c.id) FROM service_category c WHERE c.org_id=1`
	if !all {
		q += ` AND c.status='ACTIVE'`
	}
	q += ` ORDER BY c.sort_order,c.id`
	rows, e := s.db.QueryContext(ctx, q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var x Category
		var n sql.NullInt64
		var v int64
		if e = rows.Scan(&v, &n, &x.Name, &x.SortOrder, &x.Status, &x.SKUCount); e != nil {
			return nil, e
		}
		x.ID = fmt.Sprint(v)
		x.ParentID = nullable(n)
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Service) category(ctx context.Context, n int64) (Category, error) {
	var x Category
	var p sql.NullInt64
	var v int64
	e := s.db.QueryRowContext(ctx, `SELECT c.id,c.parent_id,c.name,c.sort_order,c.status,(SELECT COUNT(*) FROM service_sku x WHERE x.org_id=c.org_id AND x.category_id=c.id) FROM service_category c WHERE c.org_id=1 AND c.id=$1`, n).Scan(&v, &p, &x.Name, &x.SortOrder, &x.Status, &x.SKUCount)
	if e == sql.ErrNoRows {
		return x, httpx.E("CATEGORY_NOT_FOUND", "分类不存在", 404)
	}
	x.ID = fmt.Sprint(v)
	x.ParentID = nullable(p)
	return x, e
}
func (s *Service) categoryInput(ctx context.Context, w CategoryWrite, current int64) (sql.NullInt64, error) {
	w.Name = strings.TrimSpace(w.Name)
	if len([]rune(w.Name)) < 2 || len([]rune(w.Name)) > 64 || w.SortOrder < 0 || w.SortOrder > 9999 {
		return sql.NullInt64{}, httpx.E("VALIDATION_ERROR", "分类名称需为 2-64 字，排序需为 0-9999", 400)
	}
	if w.ParentID == nil || strings.TrimSpace(*w.ParentID) == "" {
		return sql.NullInt64{}, nil
	}
	p, e := id(*w.ParentID)
	if e != nil {
		return sql.NullInt64{}, e
	}
	if p == current {
		return sql.NullInt64{}, httpx.E("VALIDATION_ERROR", "分类不能以自身为父级", 400)
	}
	if _, e = s.category(ctx, p); e != nil {
		return sql.NullInt64{}, e
	}
	return sql.NullInt64{Int64: p, Valid: true}, nil
}
func (s *Service) CreateCategory(ctx context.Context, w CategoryWrite) (Category, error) {
	w.Name = strings.TrimSpace(w.Name)
	p, e := s.categoryInput(ctx, w, 0)
	if e != nil {
		return Category{}, e
	}
	var n int64
	e = s.db.QueryRowContext(ctx, `INSERT INTO service_category(org_id,parent_id,name,sort_order,status) VALUES(1,$1,$2,$3,'ACTIVE') RETURNING id`, p, w.Name, w.SortOrder).Scan(&n)
	if e != nil {
		return Category{}, e
	}
	return s.category(ctx, n)
}
func (s *Service) UpdateCategory(ctx context.Context, n int64, w CategoryWrite) (Category, error) {
	w.Name = strings.TrimSpace(w.Name)
	if _, e := s.category(ctx, n); e != nil {
		return Category{}, e
	}
	p, e := s.categoryInput(ctx, w, n)
	if e != nil {
		return Category{}, e
	}
	_, e = s.db.ExecContext(ctx, `UPDATE service_category SET parent_id=$1,name=$2,sort_order=$3 WHERE org_id=1 AND id=$4`, p, w.Name, w.SortOrder, n)
	if e != nil {
		return Category{}, e
	}
	return s.category(ctx, n)
}
func (s *Service) CategoryStatus(ctx context.Context, n int64, status string) (Category, error) {
	if _, e := s.category(ctx, n); e != nil {
		return Category{}, e
	}
	if status != "ACTIVE" && status != "DISABLED" {
		return Category{}, httpx.E("VALIDATION_ERROR", "分类状态错误", 400)
	}
	if status == "DISABLED" {
		var count int
		if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_sku WHERE org_id=1 AND category_id=$1 AND status='PUBLISHED'`, n).Scan(&count); e != nil {
			return Category{}, e
		}
		if count > 0 {
			return Category{}, httpx.E("CATEGORY_IN_USE", "分类下存在已发布 SKU，请先下架或移动服务", 409)
		}
	}
	_, e := s.db.ExecContext(ctx, `UPDATE service_category SET status=$1 WHERE org_id=1 AND id=$2`, status, n)
	if e != nil {
		return Category{}, e
	}
	return s.category(ctx, n)
}

func (s *Service) AdminList(ctx context.Context, key, categoryID string, page, size int) (Page[SKUAdmin], error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	q := "%" + strings.TrimSpace(key) + "%"
	var out Page[SKUAdmin]
	out.Page, out.PageSize = page, size
	if e := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_sku WHERE org_id=1 AND ($3='' OR category_id=$3::bigint) AND (name ILIKE $1 OR COALESCE(sku_code,'') ILIKE $2)`, q, q, categoryID).Scan(&out.Total); e != nil {
		return out, e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT s.id,s.sku_code,s.name,c.name,s.base_price,s.unit,s.status,s.current_published_version,s.created_at,s.updated_at,s.created_by,s.updated_by FROM service_sku s JOIN service_category c ON c.id=s.category_id AND c.org_id=s.org_id WHERE s.org_id=1 AND ($3='' OR s.category_id=$3::bigint) AND (s.name ILIKE $1 OR COALESCE(s.sku_code,'') ILIKE $2) ORDER BY s.updated_at DESC LIMIT $4 OFFSET $5`, q, q, categoryID, size, (page-1)*size)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	out.Items = []SKUAdmin{}
	for rows.Next() {
		var x SKUAdmin
		var n int64
		var pv sql.NullInt64
		if e = rows.Scan(&n, &x.SKUCode, &x.Name, &x.CategoryName, &x.BasePrice, &x.Unit, &x.Status, &pv, &x.CreatedAt, &x.UpdatedAt, &x.CreatedBy, &x.UpdatedBy); e != nil {
			return out, e
		}
		x.ID = fmt.Sprint(n)
		if pv.Valid {
			v := int(pv.Int64)
			x.PublishedVersion = &v
		}
		out.Items = append(out.Items, x)
	}
	return out, rows.Err()
}
func (s *Service) Detail(ctx context.Context, n int64) (SKUDetail, error) {
	var x SKUDetail
	var rawID, cat int64
	var cover, pv sql.NullInt64
	e := s.db.QueryRowContext(ctx, `SELECT id,category_id,sku_code,name,COALESCE(description,''),service_scope,exclusions,warranty_description,price_mode,base_price,unit,cover_media_id,status,version,current_published_version,created_by,created_at,updated_by,updated_at FROM service_sku WHERE org_id=1 AND id=$1`, n).Scan(&rawID, &cat, &x.SKUCode, &x.Name, &x.Description, &x.ServiceScope, &x.Exclusions, &x.WarrantyDescription, &x.PriceMode, &x.BasePrice, &x.Unit, &cover, &x.Status, &x.Version, &pv, &x.CreatedBy, &x.CreatedAt, &x.UpdatedBy, &x.UpdatedAt)
	if e == sql.ErrNoRows {
		return x, httpx.E("SKU_NOT_FOUND", "SKU 不存在", 404)
	}
	if e != nil {
		return x, e
	}
	x.ID, x.CategoryID = fmt.Sprint(rawID), fmt.Sprint(cat)
	x.CoverMediaID = nullable(cover)
	if pv.Valid {
		v := int(pv.Int64)
		x.PublishedVersion = &v
	}
	rows, e := s.db.QueryContext(ctx, `SELECT media_id FROM service_sku_media WHERE org_id=1 AND sku_id=$1 AND media_role='GALLERY' ORDER BY sort_order,id`, n)
	if e != nil {
		return x, e
	}
	defer rows.Close()
	x.GalleryMediaIDs = []string{}
	for rows.Next() {
		var m int64
		if e = rows.Scan(&m); e != nil {
			return x, e
		}
		x.GalleryMediaIDs = append(x.GalleryMediaIDs, fmt.Sprint(m))
	}
	reqRows, e := s.db.QueryContext(ctx, `SELECT skill_id FROM service_sku_skill_requirement WHERE org_id=1 AND sku_id=$1 ORDER BY skill_id`, n)
	if e != nil {
		return x, e
	}
	defer reqRows.Close()
	for reqRows.Next() {
		var skillID int64
		if e = reqRows.Scan(&skillID); e != nil {
			return x, e
		}
		x.RequiredSkillIDs = append(x.RequiredSkillIDs, fmt.Sprint(skillID))
	}
	return x, rows.Err()
}

var codeRE = regexp.MustCompile(`^[A-Z0-9-]{2,64}$`)

func (s *Service) validateWrite(ctx context.Context, w SKUWrite) error {
	cat, e := id(w.CategoryID)
	if e != nil {
		return e
	}
	var c int
	if e = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM service_category WHERE org_id=1 AND id=$1 AND status='ACTIVE'`, cat).Scan(&c); e != nil {
		return e
	}
	if c == 0 {
		return httpx.E("CATEGORY_NOT_FOUND", "分类不存在", 404)
	}
	w.SKUCode = strings.ToUpper(w.SKUCode)
	if !codeRE.MatchString(w.SKUCode) || len([]rune(strings.TrimSpace(w.Name))) < 2 || len([]rune(w.Name)) > 128 || w.BasePrice < 1 || w.BasePrice > 99999999 {
		return httpx.E("VALIDATION_ERROR", "SKU 编码、名称或价格不合法", 400)
	}
	if len(w.GalleryMediaIDs) > 8 {
		return httpx.E("MEDIA_COUNT_EXCEEDED", "轮播图最多 8 张", 400)
	}
	return nil
}
func duplicate(e error) bool { var p *pgconn.PgError; return errors.As(e, &p) && p.Code == "23505" }
func (s *Service) validMedia(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, n int64) bool {
	var c int
	return q.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_asset WHERE id=$1 AND org_id=1 AND purpose='SKU_IMAGE' AND status='READY'`, n).Scan(&c) == nil && c > 0
}
func (s *Service) bind(ctx context.Context, tx *sql.Tx, n int64, w SKUWrite) error {
	cover, e := id(w.CoverMediaID)
	if e != nil || !s.validMedia(ctx, tx, cover) {
		return httpx.E("SKU_COVER_REQUIRED", "封面图不可用", 400)
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM service_sku_media WHERE org_id=1 AND sku_id=$1`, n); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO service_sku_media(org_id,sku_id,media_id,media_role,sort_order) VALUES(1,$1,$2,'COVER',0)`, n, cover); e != nil {
		return e
	}
	sort := 1
	seen := map[int64]bool{cover: true}
	for _, v := range w.GalleryMediaIDs {
		m, e := id(v)
		if e != nil || !s.validMedia(ctx, tx, m) {
			return httpx.E("MEDIA_NOT_READY", "轮播图不可用", 409)
		}
		if !seen[m] {
			if _, e = tx.ExecContext(ctx, `INSERT INTO service_sku_media(org_id,sku_id,media_id,media_role,sort_order) VALUES(1,$1,$2,'GALLERY',$3)`, n, m, sort); e != nil {
				return e
			}
			sort++
			seen[m] = true
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM service_sku_skill_requirement WHERE org_id=1 AND sku_id=$1`, n); e != nil {
		return e
	}
	seenSkills := map[int64]bool{}
	for _, rawSkill := range w.RequiredSkillIDs {
		skillID, e := id(rawSkill)
		if e != nil {
			return httpx.E("WORKER_SKILL_INVALID", "SKU 技能要求不合法", 400)
		}
		if seenSkills[skillID] {
			continue
		}
		var active int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_skill WHERE org_id=1 AND id=$1 AND status='ACTIVE'`, skillID).Scan(&active); e != nil {
			return e
		}
		if active == 0 {
			return httpx.E("WORKER_SKILL_INVALID", "SKU 技能要求不可用", 400)
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO service_sku_skill_requirement(org_id,sku_id,skill_id) VALUES(1,$1,$2)`, n, skillID); e != nil {
			return e
		}
		seenSkills[skillID] = true
	}
	return nil
}
func (s *Service) Create(ctx context.Context, w SKUWrite, actor string) (SKUDetail, error) {
	if e := s.validateWrite(ctx, w); e != nil {
		return SKUDetail{}, e
	}
	cat, _ := id(w.CategoryID)
	cover, _ := id(w.CoverMediaID)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return SKUDetail{}, e
	}
	defer tx.Rollback()
	var n int64
	e = tx.QueryRowContext(ctx, `INSERT INTO service_sku(org_id,category_id,sku_code,name,description,service_scope,exclusions,warranty_description,price_mode,base_price,unit,cover_media_id,status,created_by,updated_by) VALUES(1,$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'DRAFT',$12,$12) RETURNING id`, cat, strings.ToUpper(w.SKUCode), w.Name, w.Description, w.ServiceScope, w.Exclusions, w.WarrantyDescription, w.PriceMode, w.BasePrice, w.Unit, cover, actor).Scan(&n)
	if duplicate(e) {
		return SKUDetail{}, httpx.E("SKU_CODE_DUPLICATED", "SKU 编码已存在", 409)
	}
	if e != nil {
		return SKUDetail{}, e
	}
	if e = s.bind(ctx, tx, n, w); e != nil {
		return SKUDetail{}, e
	}
	if e = tx.Commit(); e != nil {
		return SKUDetail{}, e
	}
	return s.Detail(ctx, n)
}
func (s *Service) Update(ctx context.Context, n int64, w SKUWrite, actor string) (SKUDetail, error) {
	if e := s.validateWrite(ctx, w); e != nil {
		return SKUDetail{}, e
	}
	current, e := s.Detail(ctx, n)
	if e != nil {
		return SKUDetail{}, e
	}
	if current.Version != w.Version {
		return SKUDetail{}, httpx.E("SKU_VERSION_CONFLICT", "SKU 已被其他人修改，请刷新", 409)
	}
	cat, _ := id(w.CategoryID)
	cover, _ := id(w.CoverMediaID)
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return SKUDetail{}, e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE service_sku SET category_id=$1,name=$2,description=$3,service_scope=$4,exclusions=$5,warranty_description=$6,price_mode=$7,base_price=$8,unit=$9,cover_media_id=$10,updated_by=$11,version=version+1 WHERE org_id=1 AND id=$12 AND version=$13`, cat, w.Name, w.Description, w.ServiceScope, w.Exclusions, w.WarrantyDescription, w.PriceMode, w.BasePrice, w.Unit, cover, actor, n, w.Version)
	if e != nil {
		return SKUDetail{}, e
	}
	changed, _ := r.RowsAffected()
	if changed == 0 {
		return SKUDetail{}, httpx.E("SKU_VERSION_CONFLICT", "SKU 已被其他人修改，请刷新", 409)
	}
	if e = s.bind(ctx, tx, n, w); e != nil {
		return SKUDetail{}, e
	}
	if e = tx.Commit(); e != nil {
		return SKUDetail{}, e
	}
	return s.Detail(ctx, n)
}
func (s *Service) Publish(ctx context.Context, n int64, user string) (PublishResult, error) {
	x, e := s.Detail(ctx, n)
	if e != nil {
		return PublishResult{}, e
	}
	if x.PriceMode != "FIXED" {
		return PublishResult{}, httpx.E("SKU_PRICE_MODE_NOT_SUPPORTED", "本期仅支持固定价发布", 400)
	}
	if len([]rune(strings.TrimSpace(x.ServiceScope))) < 10 || len([]rune(strings.TrimSpace(x.Exclusions))) < 5 || len([]rune(strings.TrimSpace(x.WarrantyDescription))) < 5 {
		return PublishResult{}, httpx.E("SKU_SERVICE_TERMS_REQUIRED", "请完整填写服务范围、除外项和售后/质保说明", 400)
	}
	if x.CoverMediaID == nil {
		return PublishResult{}, httpx.E("SKU_COVER_REQUIRED", "请上传有效封面图", 400)
	}
	cover, _ := id(*x.CoverMediaID)
	if !s.validMedia(ctx, s.db, cover) {
		return PublishResult{}, httpx.E("SKU_COVER_REQUIRED", "请上传有效封面图", 400)
	}
	next := 1
	if x.PublishedVersion != nil {
		next = *x.PublishedVersion + 1
	}
	urls := []string{"/api/v1/public/media/" + *x.CoverMediaID}
	for _, m := range x.GalleryMediaIDs {
		urls = append(urls, "/api/v1/public/media/"+m)
	}
	snap := PublishedSKU{x.ID, x.SKUCode, x.Name, x.Description, x.ServiceScope, x.Exclusions, x.WarrantyDescription, x.PriceMode, x.BasePrice, x.Unit, urls[0], urls, next}
	if x.PublishedVersion != nil {
		var currentRaw []byte
		var publishedAt time.Time
		e = s.db.QueryRowContext(ctx, `SELECT snapshot_json,published_at FROM service_sku_version WHERE org_id=1 AND sku_id=$1 AND version_no=$2`, n, *x.PublishedVersion).Scan(&currentRaw, &publishedAt)
		if e != nil && e != sql.ErrNoRows {
			return PublishResult{}, e
		}
		if e == nil {
			var current PublishedSKU
			if json.Unmarshal(currentRaw, &current) == nil && samePublishedSKU(current, snap) {
				return PublishResult{x.ID, "PUBLISHED", *x.PublishedVersion, publishedAt}, nil
			}
		}
	}
	raw, _ := json.Marshal(snap)
	now := time.Now().UTC()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return PublishResult{}, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO service_sku_version(org_id,sku_id,version_no,snapshot_json,published_by,published_at) VALUES(1,$1,$2,$3,$4,$5)`, n, next, raw, user, now); e != nil {
		return PublishResult{}, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE service_sku SET status='PUBLISHED',current_published_version=$1,published_at=$2 WHERE org_id=1 AND id=$3`, next, now, n); e != nil {
		return PublishResult{}, e
	}
	if e = tx.Commit(); e != nil {
		return PublishResult{}, e
	}
	return PublishResult{x.ID, "PUBLISHED", next, now}, nil
}
func (s *Service) OffShelf(ctx context.Context, n int64) error {
	r, e := s.db.ExecContext(ctx, `UPDATE service_sku SET status='OFF_SHELF' WHERE org_id=1 AND id=$1`, n)
	if e != nil {
		return e
	}
	c, _ := r.RowsAffected()
	if c == 0 {
		return httpx.E("SKU_NOT_FOUND", "SKU 不存在", 404)
	}
	return nil
}
func (s *Service) PublicList(ctx context.Context, key string) ([]PublishedSKU, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT v.snapshot_json FROM service_sku s JOIN service_sku_version v ON v.org_id=s.org_id AND v.sku_id=s.id AND v.version_no=s.current_published_version WHERE s.org_id=1 AND s.status='PUBLISHED' ORDER BY s.published_at DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []PublishedSKU{}
	key = strings.ToLower(strings.TrimSpace(key))
	for rows.Next() {
		var raw []byte
		var x PublishedSKU
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &x); e != nil {
			return nil, e
		}
		if key == "" || strings.Contains(strings.ToLower(x.Name), key) || strings.Contains(strings.ToLower(x.Description), key) {
			out = append(out, x)
		}
	}
	return out, rows.Err()
}
func (s *Service) PublicDetail(ctx context.Context, n int64) (PublishedSKU, error) {
	var raw []byte
	var x PublishedSKU
	e := s.db.QueryRowContext(ctx, `SELECT v.snapshot_json FROM service_sku s JOIN service_sku_version v ON v.org_id=s.org_id AND v.sku_id=s.id AND v.version_no=s.current_published_version WHERE s.org_id=1 AND s.id=$1 AND s.status='PUBLISHED'`, n).Scan(&raw)
	if e == sql.ErrNoRows {
		return x, httpx.E("SKU_NOT_AVAILABLE", "服务未发布或已下架", 404)
	}
	if e != nil {
		return x, e
	}
	e = json.Unmarshal(raw, &x)
	return x, e
}
func (s *Service) PublicGroups(ctx context.Context) ([]CategoryGroup, error) {
	cats, e := s.Categories(ctx, false)
	if e != nil {
		return nil, e
	}
	out := make([]CategoryGroup, 0, len(cats))
	for _, c := range cats {
		rows, e := s.db.QueryContext(ctx, `SELECT v.snapshot_json FROM service_sku s JOIN service_sku_version v ON v.org_id=s.org_id AND v.sku_id=s.id AND v.version_no=s.current_published_version WHERE s.org_id=1 AND s.category_id=$1 AND s.status='PUBLISHED' ORDER BY s.published_at DESC`, c.ID)
		if e != nil {
			return nil, e
		}
		g := CategoryGroup{c.ID, c.Name, []PublishedSKU{}}
		for rows.Next() {
			var raw []byte
			var x PublishedSKU
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return nil, e
			}
			if e = json.Unmarshal(raw, &x); e != nil {
				rows.Close()
				return nil, e
			}
			g.Services = append(g.Services, x)
		}
		rows.Close()
		out = append(out, g)
	}
	return out, nil
}

type Handler struct{ s *Service }

func NewHandler(s *Service) *Handler { return &Handler{s} }
func fail(w http.ResponseWriter, r *http.Request, v any, e error) {
	if e != nil {
		httpx.Failure(w, r, e)
	} else {
		httpx.Success(w, r, v)
	}
}
func pageInt(r *http.Request, k string, d int) int {
	var n int
	fmt.Sscan(r.URL.Query().Get(k), &n)
	if n == 0 {
		return d
	}
	return n
}
func (h *Handler) Categories(w http.ResponseWriter, r *http.Request) {
	v, e := h.s.Categories(r.Context(), r.URL.Query().Get("includeDisabled") == "true")
	fail(w, r, v, e)
}
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var b CategoryWrite
	if e := httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.CreateCategory(r.Context(), b)
	fail(w, r, v, e)
}
func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var b CategoryWrite
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.UpdateCategory(r.Context(), n, b)
	fail(w, r, v, e)
}
func (h *Handler) CategoryStatus(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var b struct {
		Status string `json:"status"`
	}
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.CategoryStatus(r.Context(), n, b.Status)
	fail(w, r, v, e)
}
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	v, e := h.s.AdminList(r.Context(), r.URL.Query().Get("keyword"), r.URL.Query().Get("categoryId"), pageInt(r, "page", 1), pageInt(r, "pageSize", 20))
	fail(w, r, v, e)
}
func (h *Handler) AdminDetail(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Detail(r.Context(), n)
	fail(w, r, v, e)
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	var b SKUWrite
	if e := httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Create(r.Context(), b, p.Name)
	fail(w, r, v, e)
}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.From(r.Context())
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	var b SKUWrite
	if e = httpx.DecodeJSON(w, r, &b); e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Update(r.Context(), n, b, p.Name)
	fail(w, r, v, e)
}
func (h *Handler) Publish(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.Publish(r.Context(), n, "admin")
	fail(w, r, v, e)
}
func (h *Handler) OffShelf(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e == nil {
		e = h.s.OffShelf(r.Context(), n)
	}
	fail(w, r, nil, e)
}
func (h *Handler) PublicList(w http.ResponseWriter, r *http.Request) {
	v, e := h.s.PublicList(r.Context(), r.URL.Query().Get("keyword"))
	fail(w, r, v, e)
}
func (h *Handler) PublicDetail(w http.ResponseWriter, r *http.Request) {
	n, e := httpx.PathID(r, "id")
	if e != nil {
		httpx.Failure(w, r, e)
		return
	}
	v, e := h.s.PublicDetail(r.Context(), n)
	fail(w, r, v, e)
}
func (h *Handler) PublicGroups(w http.ResponseWriter, r *http.Request) {
	v, e := h.s.PublicGroups(r.Context())
	fail(w, r, v, e)
}
