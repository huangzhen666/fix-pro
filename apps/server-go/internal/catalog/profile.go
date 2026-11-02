package catalog

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/fixpro/server/internal/platform/httpx"
	"strings"
)

type QuantityRule struct {
	Min         int    `json:"min"`
	Max         int    `json:"max"`
	Step        int    `json:"step"`
	Aggregation string `json:"aggregation"`
}
type Billing struct {
	UnitDefinition      string       `json:"unitDefinition"`
	Quantity            QuantityRule `json:"quantity"`
	QuantityExplanation string       `json:"quantityExplanation"`
	VisitFeeMode        string       `json:"visitFeeMode"`
	FeeDescription      string       `json:"feeDescription"`
}
type ServiceItem struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	IconKey      string `json:"iconKey,omitempty"`
	ImageMediaID string `json:"imageMediaId,omitempty"`
}
type ExcludedItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}
type StepItem struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	IconKey     string `json:"iconKey,omitempty"`
}
type PolicyPeriod struct {
	Value            int    `json:"value"`
	Unit             string `json:"unit"`
	StartDescription string `json:"startDescription"`
}
type AfterSales struct {
	Type               string        `json:"type"`
	Summary            string        `json:"summary"`
	Coverage           string        `json:"coverage"`
	Exclusions         []string      `json:"exclusions,omitempty"`
	Period             *PolicyPeriod `json:"period,omitempty"`
	ContactDescription string        `json:"contactDescription"`
}
type CollectionField struct {
	Mode  string `json:"mode"`
	Label string `json:"label"`
	Hint  string `json:"hint"`
}
type DemandCollection struct {
	Description CollectionField `json:"description"`
	Media       CollectionField `json:"media"`
}
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Resource struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}
type Photo struct {
	MediaID string `json:"mediaId"`
	Caption string `json:"caption"`
}
type Comparison struct {
	Title         string `json:"title"`
	BeforeMediaID string `json:"beforeMediaId"`
	AfterMediaID  string `json:"afterMediaId"`
	Description   string `json:"description"`
	EffectNotice  string `json:"effectNotice"`
}
type FAQ struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type Enhancements struct {
	Tags         []string     `json:"tags,omitempty"`
	Facts        []Fact       `json:"facts,omitempty"`
	Resources    []Resource   `json:"resources,omitempty"`
	Preparations []string     `json:"preparations,omitempty"`
	Steps        []StepItem   `json:"steps,omitempty"`
	Gallery      []Photo      `json:"gallery,omitempty"`
	Comparisons  []Comparison `json:"comparisons,omitempty"`
	FAQs         []FAQ        `json:"faqs,omitempty"`
}
type Profile struct {
	SchemaVersion        int              `json:"schemaVersion"`
	TemplateKey          string           `json:"templateKey,omitempty"`
	ServiceTarget        string           `json:"serviceTarget"`
	Billing              Billing          `json:"billing"`
	IncludedItems        []ServiceItem    `json:"includedItems"`
	ExcludedItems        []ExcludedItem   `json:"excludedItems,omitempty"`
	UnsuitableConditions []string         `json:"unsuitableConditions,omitempty"`
	PurchaseNotices      []string         `json:"purchaseNotices,omitempty"`
	AcceptanceCriteria   []string         `json:"acceptanceCriteria"`
	AfterSales           AfterSales       `json:"afterSales"`
	DemandCollection     DemandCollection `json:"demandCollection"`
	Enhancements         *Enhancements    `json:"enhancements,omitempty"`
}

func (p *Profile) UnmarshalJSON(b []byte) error {
	type plain Profile
	var v plain
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(&v); e != nil {
		return e
	}
	*p = Profile(v)
	return nil
}
func (p *Profile) Scan(v any) error {
	switch b := v.(type) {
	case []byte:
		return json.Unmarshal(b, p)
	case string:
		return json.Unmarshal([]byte(b), p)
	}
	return fmt.Errorf("invalid profile")
}
func (p Profile) Value() (driver.Value, error) { b, e := json.Marshal(p); return string(b), e }
func invalid(path, why string) error {
	return httpx.E("SKU_DETAIL_PROFILE_INVALID", path+": "+why, 400)
}
func (p Profile) Validate(publish bool) error {
	if p.SchemaVersion != 1 {
		return httpx.E("SKU_DETAIL_SCHEMA_NOT_SUPPORTED", "detailProfile.schemaVersion 必须为1", 400)
	}
	b, _ := json.Marshal(p)
	if len(b) > 65536 {
		return invalid("detailProfile", "不能超过64KiB")
	}
	text := func(path, v string, min, max int, required bool) error {
		n := len([]rune(strings.TrimSpace(v)))
		if n == 0 && !required {
			return nil
		}
		if n < min || n > max {
			return invalid(path, fmt.Sprintf("需为%d–%d字", min, max))
		}
		return nil
	}
	enum := func(path, v string, required bool, choices ...string) error {
		if v == "" && !required {
			return nil
		}
		for _, c := range choices {
			if c == v {
				return nil
			}
		}
		return invalid(path, "选项不合法")
	}
	for _, f := range []struct {
		k, v     string
		min, max int
		req      bool
	}{{"templateKey", p.TemplateKey, 1, 64, false}, {"serviceTarget", p.ServiceTarget, 5, 150, publish}, {"billing.unitDefinition", p.Billing.UnitDefinition, 5, 120, publish}, {"billing.quantityExplanation", p.Billing.QuantityExplanation, 5, 200, publish}, {"billing.feeDescription", p.Billing.FeeDescription, 5, 200, publish}} {
		if e := text(f.k, f.v, f.min, f.max, f.req); e != nil {
			return e
		}
	}

	q := p.Billing.Quantity
	if publish {
		if q.Min < 1 || q.Max < q.Min || q.Max > 9999 || q.Step < 1 || q.Step > 9999 {
			return invalid("billing.quantity", "数量范围或步长不合法")
		}
	} else if q.Min < 0 || q.Min > 9999 || q.Max < 0 || q.Max > 9999 || q.Step < 0 || q.Step > 9999 || (q.Min > 0 && q.Max > 0 && q.Max < q.Min) {
		return invalid("billing.quantity", "数量范围或步长不合法")
	}
	if e := enum("billing.quantity.aggregation", q.Aggregation, publish, "INDEPENDENT", "ADDITIVE"); e != nil {
		return e
	}

	if e := enum("billing.visitFeeMode", p.Billing.VisitFeeMode, publish, "INCLUDED", "WAIVED", "NOT_APPLICABLE"); e != nil {
		return e
	}
	items := func(path string, rows []ServiceItem, min, max int) error {
		if len(rows) > max || (publish && len(rows) < min) {
			return invalid(path, "条目数量不合法")
		}
		for i, v := range rows {
			k := fmt.Sprintf("%s.%d", path, i)
			if e := text(k+".title", v.Title, 2, 30, publish); e != nil {
				return e
			}
			if e := text(k+".description", v.Description, 5, 200, publish); e != nil {
				return e
			}
			if e := enum(k+".iconKey", v.IconKey, false, "service", "inspect", "protect", "tool", "clean", "install", "report", "confirm"); e != nil {
				return e
			}
		}
		return nil
	}
	if e := items("includedItems", p.IncludedItems, 1, 12); e != nil {
		return e
	}
	excluded := make([]ServiceItem, len(p.ExcludedItems))
	for i, v := range p.ExcludedItems {
		excluded[i] = ServiceItem{Title: v.Title, Description: v.Description}
	}
	if e := items("excludedItems", excluded, 0, 12); e != nil {
		return e
	}
	stringsCheck := func(path string, vs []string, minCount, maxCount, min, max int) error {
		if len(vs) > maxCount || (publish && len(vs) < minCount) {
			return invalid(path, "条目数量不合法")
		}
		for i, v := range vs {
			if e := text(fmt.Sprintf("%s.%d", path, i), v, min, max, publish); e != nil {
				return e
			}
		}
		return nil
	}
	for _, v := range []struct {
		k                    string
		vs                   []string
		minc, maxc, min, max int
	}{{"unsuitableConditions", p.UnsuitableConditions, 0, 8, 2, 100}, {"purchaseNotices", p.PurchaseNotices, 0, 8, 5, 150}, {"acceptanceCriteria", p.AcceptanceCriteria, 1, 8, 5, 150}, {"afterSales.exclusions", p.AfterSales.Exclusions, 0, 8, 5, 150}} {
		if e := stringsCheck(v.k, v.vs, v.minc, v.maxc, v.min, v.max); e != nil {
			return e
		}
	}
	a := p.AfterSales
	if e := enum("afterSales.type", a.Type, publish, "WARRANTY", "REWORK", "SUPPORT"); e != nil {
		return e
	}
	for _, f := range []struct {
		k, v string
		max  int
	}{{"summary", a.Summary, 120}, {"coverage", a.Coverage, 300}, {"contactDescription", a.ContactDescription, 150}} {
		if e := text("afterSales."+f.k, f.v, 5, f.max, publish); e != nil {
			return e
		}
	}
	if publish && a.Type == "WARRANTY" && a.Period == nil {
		return invalid("afterSales.period", "期限质保必须填写期限")
	}
	if a.Period != nil {
		v := a.Period
		if (publish || v.Value != 0) && (v.Value < 1 || v.Value > 3650) {
			return invalid("afterSales.period.value", "需为1–3650")
		}
		if e := enum("afterSales.period.unit", v.Unit, publish, "HOUR", "DAY", "MONTH"); e != nil {
			return e
		}
		if e := text("afterSales.period.startDescription", v.StartDescription, 5, 100, publish); e != nil {
			return e
		}
	}
	for k, v := range map[string]CollectionField{"description": p.DemandCollection.Description, "media": p.DemandCollection.Media} {
		if e := enum("demandCollection."+k+".mode", v.Mode, publish, "HIDDEN", "OPTIONAL", "REQUIRED"); e != nil {
			return e
		}
		req := publish && v.Mode != "HIDDEN"
		if e := text("demandCollection."+k+".label", v.Label, 2, 20, req); e != nil {
			return e
		}
		if e := text("demandCollection."+k+".hint", v.Hint, 5, 150, req); e != nil {
			return e
		}
	}
	h := p.Enhancements
	if h == nil {
		return nil
	}
	if e := stringsCheck("enhancements.tags", h.Tags, 0, 6, 2, 16); e != nil {
		return e
	}
	if e := stringsCheck("enhancements.preparations", h.Preparations, 0, 8, 5, 150); e != nil {
		return e
	}
	if len(h.Facts) > 8 || len(h.Resources) > 8 || len(h.Gallery) > 6 || len(h.Comparisons) > 3 || len(h.FAQs) > 6 {
		return invalid("enhancements", "条目数量超限")
	}
	steps := make([]ServiceItem, len(h.Steps))
	for i, v := range h.Steps {
		steps[i] = ServiceItem{Title: v.Title, Description: v.Description, IconKey: v.IconKey}
	}
	if e := items("enhancements.steps", steps, 0, 6); e != nil {
		return e
	}
	if publish && len(h.Steps) == 1 {
		return invalid("enhancements.steps", "流程至少两步")
	}
	for i, v := range h.Facts {
		if e := text(fmt.Sprintf("enhancements.facts.%d.label", i), v.Label, 2, 20, publish); e != nil {
			return e
		}
		if e := text(fmt.Sprintf("enhancements.facts.%d.value", i), v.Value, 1, 100, publish); e != nil {
			return e
		}
	}
	for i, v := range h.Resources {
		k := fmt.Sprintf("enhancements.resources.%d", i)
		if e := text(k+".name", v.Name, 2, 30, publish); e != nil {
			return e
		}
		if e := text(k+".description", v.Description, 5, 150, publish); e != nil {
			return e
		}
		if e := enum(k+".provider", v.Provider, publish, "PROVIDER", "CUSTOMER", "SHARED"); e != nil {
			return e
		}
	}
	for i, v := range h.Gallery {
		if publish && v.MediaID == "" {
			return invalid(fmt.Sprintf("enhancements.gallery.%d.mediaId", i), "请上传图片")
		}
		if e := text("enhancements.gallery.caption", v.Caption, 5, 100, publish); e != nil {
			return e
		}
	}
	for i, v := range h.Comparisons {
		k := fmt.Sprintf("enhancements.comparisons.%d", i)
		if publish && (v.BeforeMediaID == "" || v.AfterMediaID == "" || v.BeforeMediaID == v.AfterMediaID) {
			return invalid(k, "请上传不同的前后两张图片")
		}
		for _, f := range []struct {
			k, v     string
			min, max int
		}{{"title", v.Title, 2, 30}, {"description", v.Description, 5, 200}, {"effectNotice", v.EffectNotice, 5, 150}} {
			if e := text(k+"."+f.k, f.v, f.min, f.max, publish); e != nil {
				return e
			}
		}
	}
	for _, v := range h.FAQs {
		if e := text("enhancements.faqs.question", v.Question, 5, 60, publish); e != nil {
			return e
		}
		if e := text("enhancements.faqs.answer", v.Answer, 5, 300, publish); e != nil {
			return e
		}
	}
	return nil
}
func (p Profile) CheckQuantity(n int) error {
	q := p.Billing.Quantity
	if n < 1 || n > 9999 || q.Min < 1 || q.Step < 1 || n < q.Min || n > q.Max || (n-q.Min)%q.Step != 0 {
		return httpx.E("SKU_QUANTITY_INVALID", fmt.Sprintf("数量需为%d–%d，步长%d", q.Min, q.Max, q.Step), 400)
	}
	return nil
}
func (p Profile) CheckDemand(desc string, media int) error {
	if len([]rune(strings.TrimSpace(desc))) > 500 {
		return httpx.E("VALIDATION_ERROR", "需求说明最多500字", 400)
	}
	if (p.DemandCollection.Description.Mode == "REQUIRED" && len([]rune(strings.TrimSpace(desc))) < 5) || (p.DemandCollection.Media.Mode == "REQUIRED" && media < 1) {
		return httpx.E("ORDER_DEMAND_REQUIRED", "请按服务要求补充需求说明和现场资料", 400)
	}
	return nil
}
func (p Profile) MediaIDs() []string {
	out := []string{}
	for _, v := range p.IncludedItems {
		if v.ImageMediaID != "" {
			out = append(out, v.ImageMediaID)
		}
	}
	if p.Enhancements != nil {
		for _, v := range p.Enhancements.Gallery {
			if v.MediaID != "" {
				out = append(out, v.MediaID)
			}
		}
		for _, v := range p.Enhancements.Comparisons {
			if v.BeforeMediaID != "" {
				out = append(out, v.BeforeMediaID)
			}
			if v.AfterMediaID != "" {
				out = append(out, v.AfterMediaID)
			}
		}
	}
	return out
}

// Canonicalize absent optional content so untouched form fields do not create new publications.
func NormalizeProfile(p Profile) Profile {
	if len(p.ExcludedItems) == 0 {
		p.ExcludedItems = nil
	}
	if len(p.UnsuitableConditions) == 0 {
		p.UnsuitableConditions = nil
	}
	if len(p.PurchaseNotices) == 0 {
		p.PurchaseNotices = nil
	}
	if len(p.AfterSales.Exclusions) == 0 {
		p.AfterSales.Exclusions = nil
	}
	if p.Enhancements != nil {
		h := *p.Enhancements
		if len(h.Tags) == 0 {
			h.Tags = nil
		}
		if len(h.Facts) == 0 {
			h.Facts = nil
		}
		if len(h.Resources) == 0 {
			h.Resources = nil
		}
		if len(h.Preparations) == 0 {
			h.Preparations = nil
		}
		if len(h.Steps) == 0 {
			h.Steps = nil
		}
		if len(h.Gallery) == 0 {
			h.Gallery = nil
		}
		if len(h.Comparisons) == 0 {
			h.Comparisons = nil
		}
		if len(h.FAQs) == 0 {
			h.FAQs = nil
		}
		if len(h.Tags)+len(h.Facts)+len(h.Resources)+len(h.Preparations)+len(h.Steps)+len(h.Gallery)+len(h.Comparisons)+len(h.FAQs) == 0 {
			p.Enhancements = nil
		} else {
			p.Enhancements = &h
		}
	}
	return p
}
