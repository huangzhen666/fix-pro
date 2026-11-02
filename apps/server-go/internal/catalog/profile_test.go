package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfilesAcrossServices(t *testing.T) {
	for _, v := range Examples() {
		t.Run(v.Key, func(t *testing.T) {
			if e := v.Profile.Validate(true); e != nil {
				t.Fatal(e)
			}
			q := v.Profile.Billing.Quantity
			if e := v.Profile.CheckQuantity(q.Min); e != nil {
				t.Fatal(e)
			}
			if e := v.Profile.CheckQuantity(q.Max + 1); e == nil {
				t.Fatal("accepted excess quantity")
			}
			if v.Key == "DEMO-CLEAN" && v.Profile.CheckDemand("", 0) != nil {
				t.Fatal("cleaning requires fault evidence")
			}
		})
	}
}
func TestProfileDraftStrictFieldsAndPolicies(t *testing.T) {
	var draft Profile
	if e := json.Unmarshal([]byte(`{"schemaVersion":1}`), &draft); e != nil {
		t.Fatal(e)
	}
	if e := draft.Validate(false); e != nil {
		t.Fatal(e)
	}
	if e := draft.Validate(true); e == nil {
		t.Fatal("incomplete draft published")
	}
	for _, s := range []string{`{"schemaVersion":1,"modules":[]}`, `{"schemaVersion":1,"billing":{"layout":"custom"}}`, `{"schemaVersion":1,"excludedItems":[{"title":"额外服务","description":"不在本次服务范围","imageMediaId":"1"}]}`} {
		if e := json.Unmarshal([]byte(s), &draft); e == nil {
			t.Fatal("unknown fields accepted")
		}
	}
	p := Examples()[0].Profile
	p.AfterSales.Period = nil
	if p.Validate(true) == nil {
		t.Fatal("warranty without period")
	}
	p.AfterSales.Type = "REWORK"
	if e := p.Validate(true); e != nil {
		t.Fatal(e)
	}
	p.ServiceTarget = strings.Repeat("界", 151)
	if p.Validate(false) == nil {
		t.Fatal("unicode length unchecked")
	}
}
func TestQuantityStepAndDemand(t *testing.T) {
	p := Examples()[1].Profile
	p.Billing.Quantity = QuantityRule{2, 8, 2, "ADDITIVE"}
	for _, n := range []int{0, 1, 3, 9, 10000} {
		if p.CheckQuantity(n) == nil {
			t.Fatalf("accepted %d", n)
		}
	}
	for _, n := range []int{2, 4, 8} {
		if e := p.CheckQuantity(n); e != nil {
			t.Fatal(e)
		}
	}
	p.DemandCollection.Media.Mode = "REQUIRED"
	if p.CheckDemand("", 0) == nil {
		t.Fatal("required media not enforced")
	}
	if e := p.CheckDemand("", 1); e != nil {
		t.Fatal(e)
	}
}
func TestEnhancementsAreOptionalAndValidated(t *testing.T) {
	p := Examples()[1].Profile
	if e := p.Validate(true); e != nil {
		t.Fatal(e)
	}
	p.Enhancements = &Enhancements{Steps: []StepItem{{Title: "现场确认", Description: "与客户核对本次服务范围"}}}
	if p.Validate(true) == nil {
		t.Fatal("one step accepted")
	}
	p.Enhancements = &Enhancements{Comparisons: []Comparison{{Title: "清洁效果", BeforeMediaID: "1", AfterMediaID: "1", Description: "展示约定区域的清洁效果", EffectNotice: "效果取决于实际现场情况"}}}
	if p.Validate(true) == nil {
		t.Fatal("duplicate comparison image")
	}
}

func TestEmptyEnhancementsDoNotChangePublication(t *testing.T) {
	a := PublishedSKU{DetailProfile: Examples()[1].Profile}
	b := a
	b.DetailProfile.Enhancements = &Enhancements{Tags: []string{}, Gallery: []Photo{}}
	if !samePublishedSKU(a, b) {
		t.Fatal("empty UI fields created a publication change")
	}
}
