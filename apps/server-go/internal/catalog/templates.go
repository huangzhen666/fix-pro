package catalog

import "net/http"

type Template struct {
	Key        string        `json:"key"`
	Name       string        `json:"name"`
	Unit       string        `json:"unit"`
	Profile    Profile       `json:"profile"`
	Candidates []ServiceItem `json:"candidates"`
}

func Templates() []Template {
	base := Profile{SchemaVersion: 1, Billing: Billing{Quantity: QuantityRule{1, 99, 1, "INDEPENDENT"}, QuantityExplanation: "每份对应一个独立服务对象，按约定范围提供服务", VisitFeeMode: "INCLUDED", FeeDescription: "套餐价格包含本次上门服务费用，不另收上门费"}, IncludedItems: []ServiceItem{}, AcceptanceCriteria: []string{"完成约定服务后与客户逐项检查确认"}, AfterSales: AfterSales{Type: "SUPPORT", Summary: "如对本次服务有疑问，请联系客服核查", Coverage: "对本次约定服务内容提供咨询及问题核查", ContactDescription: "请通过平台客服提供订单号和问题说明"}, DemandCollection: DemandCollection{CollectionField{"OPTIONAL", "服务需求", "可补充服务对象和特别需要注意的情况"}, CollectionField{"OPTIONAL", "现场资料", "可上传现场照片或视频，便于了解服务情况"}}}
	rows := []Template{{Key: "GENERAL_SERVICE_V1", Name: "通用服务", Unit: "份", Profile: base, Candidates: []ServiceItem{}}, {Key: "WALL_PAINT_PATCH_V1", Name: "墙面补漆", Unit: "处", Profile: base, Candidates: []ServiceItem{{Title: "周边保护", Description: "对约定作业区域进行必要遮挡保护", IconKey: "protect"}, {Title: "表面处理", Description: "在约定基层条件下进行局部表面处理", IconKey: "tool"}, {Title: "局部涂刷", Description: "在确认材料和颜色后进行约定部位涂刷", IconKey: "service"}, {Title: "现场清理", Description: "清理本次施工产生的作业垃圾", IconKey: "clean"}}}, {Key: "HOME_CLEANING_V1", Name: "日常保洁", Unit: "小时", Profile: base, Candidates: []ServiceItem{{Title: "厨房表面", Description: "清洁约定厨房区域可触达的外部表面", IconKey: "clean"}, {Title: "卫生间清洁", Description: "清洁约定卫生间区域的常规表面", IconKey: "clean"}, {Title: "地面清洁", Description: "清洁约定服务区域可触达的地面", IconKey: "clean"}}}, {Key: "DEVICE_INSPECTION_V1", Name: "设备检测", Unit: "台", Profile: base, Candidates: []ServiceItem{{Title: "约定项目检测", Description: "按事先约定的设备类型和项目进行检测", IconKey: "inspect"}, {Title: "结果说明", Description: "向客户说明本次检测结果和处理建议", IconKey: "report"}}}}
	for i := range rows {
		p := &rows[i].Profile
		p.TemplateKey = rows[i].Key
		switch rows[i].Key {
		case "HOME_CLEANING_V1":
			p.DemandCollection.Description.Label = "清洁需求"
			p.DemandCollection.Description.Hint = "可说明重点清洁区域和特别注意事项"
			p.Billing.Quantity = QuantityRule{2, 8, 1, "ADDITIVE"}
			p.Billing.QuantityExplanation = "数量为一名人员的累计服务小时，不自动增加人员"
		case "WALL_PAINT_PATCH_V1":
			p.DemandCollection.Description.Label = "问题描述"
			p.DemandCollection.Media.Label = "现场照片"
			p.DemandCollection.Description.Mode = "REQUIRED"
			p.DemandCollection.Media.Mode = "REQUIRED"
		case "DEVICE_INSPECTION_V1":
			p.DemandCollection.Description.Label = "设备信息"
			p.DemandCollection.Description.Hint = "请填写设备型号、编号和希望检测的项目"
			p.DemandCollection.Description.Mode = "REQUIRED"
		}
	}
	return rows
}
func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) { fail(w, r, Templates(), nil) }
