package address

import "testing"

func TestValidate(t *testing.T) {
	base := Write{City: "汉川市", DetailAddress: "测试路", BuildingDoor: "1-1101", ContactName: "张三", ContactMobile: "13800138000"}
	if err := validate(base); err != nil {
		t.Fatalf("valid address rejected: %v", err)
	}
	for name, item := range map[string]Write{
		"missing city":         func() Write { x := base; x.City = ""; return x }(),
		"outside service city": func() Write { x := base; x.City = "杭州市"; return x }(),
		"missing building":     func() Write { x := base; x.BuildingDoor = ""; return x }(),
		"invalid mobile":       func() Write { x := base; x.ContactMobile = "123"; return x }(),
		"short contact name":   func() Write { x := base; x.ContactName = "张"; return x }(),
		"short detail address": func() Write { x := base; x.DetailAddress = ""; return x }(),
	} {
		if err := validate(item); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
}
