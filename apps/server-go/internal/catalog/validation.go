package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/fixpro/server/internal/platform/httpx"
	"strings"
)

type FieldIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func issuesError(issues []FieldIssue) error {
	if len(issues) == 0 {
		return nil
	}
	e := httpx.E("SKU_VALIDATION_FAILED", fmt.Sprintf("请检查%d项配置", len(issues)), 400)
	e.Details = struct {
		Errors []FieldIssue `json:"errors"`
	}{issues}
	return e
}
func normalizeWrite(w SKUWrite) SKUWrite {
	w.Name = strings.TrimSpace(w.Name)
	w.Unit = strings.TrimSpace(w.Unit)
	w.ServiceDescription = strings.TrimSpace(w.ServiceDescription)
	w.ServiceNotice = strings.TrimSpace(w.ServiceNotice)
	w.CategoryID = strings.TrimSpace(w.CategoryID)
	w.CoverMediaID = strings.TrimSpace(w.CoverMediaID)
	if w.PriceMode == "" {
		w.PriceMode = "FIXED"
	}
	return w
}
func (s *Service) validateSKU(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, w SKUWrite, publish bool) error {
	var issues []FieldIssue
	add := func(field, msg string) { issues = append(issues, FieldIssue{field, msg}) }
	for _, v := range []struct {
		field, label, value string
		max                 int
		required            bool
	}{{"name", "服务名称", w.Name, 60, true}, {"unit", "单位", w.Unit, 8, true}, {"serviceDescription", "服务说明", w.ServiceDescription, 5000, true}, {"serviceNotice", "服务须知", w.ServiceNotice, 3000, false}} {
		n := len([]rune(strings.TrimSpace(v.value)))
		if publish && v.required && n == 0 {
			add(v.field, "请填写"+v.label)
		} else if n > v.max {
			add(v.field, fmt.Sprintf("%s最多%d字", v.label, v.max))
		}
	}
	if w.BasePrice < 0 || w.BasePrice > 99999999 || (publish && w.BasePrice == 0) {
		add("basePrice", "请填写有效价格（0.01～999999.99元）")
	}
	if w.PriceMode != "FIXED" {
		add("basePrice", "仅支持固定价格")
	}
	if w.CategoryID == "" {
		if publish {
			add("categoryId", "请选择服务分类")
		}
	} else {
		n, e := id(w.CategoryID)
		var count int
		if e != nil {
			add("categoryId", "请选择有效服务分类")
		} else if e = q.QueryRowContext(ctx, `SELECT count(*) FROM service_category WHERE org_id=1 AND id=$1 AND status='ACTIVE'`, n).Scan(&count); e != nil {
			return e
		} else if count == 0 {
			add("categoryId", "分类已停用或不存在，请重新选择")
		}
	}
	if w.CoverMediaID == "" {
		if publish {
			add("coverMediaId", "请上传服务主图")
		}
	} else {
		n, e := id(w.CoverMediaID)
		if e != nil || !s.validMedia(ctx, q, n) {
			add("coverMediaId", "主图不可用，请重新上传")
		}
	}
	if len(w.GalleryMediaIDs) > 5 {
		add("galleryMediaIds", "附图最多5张")
	}
	for i, raw := range w.GalleryMediaIDs {
		n, e := id(raw)
		if e != nil || !s.validMedia(ctx, q, n) {
			add(fmt.Sprintf("galleryMediaIds.%d", i), fmt.Sprintf("第%d张附图不可用，请重新上传", i+1))
		}
	}
	return issuesError(issues)
}
func CheckQuantity(n int) error {
	if n < 1 || n > 9999 {
		return httpx.E("SKU_QUANTITY_INVALID", "数量需为1～9999的整数", 400)
	}
	return nil
}
func CheckDemand(desc string, media int) error {
	if len([]rune(strings.TrimSpace(desc))) > 500 {
		return httpx.E("VALIDATION_ERROR", "补充需求最多500字", 400)
	}
	return nil
}
func excerpt(s string) string {
	r := []rune(s)
	if len(r) > 100 {
		return string(r[:100]) + "…"
	}
	return s
}
