package catalog

// Examples are explicit test/development fixtures. They are never published at startup.
type Example struct {
	Key, Name, Unit string
	Price           int64
	Profile         Profile
}

func Examples() []Example {
	ts := Templates()
	out := []Example{}
	for _, v := range []struct {
		index                               int
		key, name, unit, definition, target string
		price                               int64
		q                                   QuantityRule
	}{{1, "DEMO-WALL", "墙面局部补漆", "处", "一处连续墙面修补，面积不超过1平方米", "基层牢固干燥的室内乳胶漆墙面", 10000, QuantityRule{1, 20, 1, "INDEPENDENT"}}, {2, "DEMO-CLEAN", "日常保洁", "小时", "一名服务人员工作一小时，按累计人工小时购买", "家庭约定区域内可触达的常规表面", 5000, QuantityRule{2, 8, 1, "ADDITIVE"}}, {0, "DEMO-AREA", "开荒保洁", "㎡", "按已确认建筑面积口径计算的一个平方米", "装修后约定区域的基础清洁服务", 600, QuantityRule{50, 300, 1, "ADDITIVE"}}, {3, "DEMO-INSPECT", "设备检测", "台", "一台约定类型设备的一次基础检测", "双方已确认型号和检测范围的设备", 8000, QuantityRule{1, 10, 1, "INDEPENDENT"}}} {
		t := ts[v.index]
		p := t.Profile
		p.Billing.Quantity = v.q
		p.ServiceTarget = v.target
		p.Billing.UnitDefinition = v.definition
		p.IncludedItems = t.Candidates
		if len(p.IncludedItems) == 0 {
			p.IncludedItems = []ServiceItem{{Title: "约定区域清洁", Description: "按约定面积和清洁标准完成基础服务", IconKey: "clean"}}
		}
		p.Billing.QuantityExplanation = "按已确认单份边界购买，不包含另行约定的服务"
		if v.q.Aggregation == "ADDITIVE" {
			p.Billing.QuantityExplanation = "数量代表累计服务额度，不自动增加服务人员"
		}
		if v.index == 1 {
			p.AfterSales = AfterSales{Type: "WARRANTY", Summary: "本次施工部位约定问题提供期限保障", Coverage: "因本次施工原因引起的约定部位起皮脱落", Period: &PolicyPeriod{90, "DAY", "从客户验收通过之日起计算"}, ContactDescription: "请联系客服提供订单号和问题照片"}
			p.PurchaseNotices = []string{"局部补漆可能存在色差，施工前确认材料和效果"}
		}
		if v.index == 2 || v.key == "DEMO-AREA" {
			p.AfterSales.Type = "REWORK"
			p.AfterSales.Summary = "对约定范围内的清洁问题提供核查处理"
		}
		out = append(out, Example{v.key, v.name, v.unit, v.price, p})
	}
	return out
}
