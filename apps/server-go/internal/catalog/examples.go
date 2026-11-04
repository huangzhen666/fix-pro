package catalog

// Explicit test examples. Never published at startup.
type Example struct {
	Key, Name, Unit, ServiceDescription, ServiceNotice string
	Price                                              int64
}

func Examples() []Example {
	return []Example{
		{"SIMPLE-WALL", "墙面局部补漆", "处", "室内局部补漆，包含保护、轻微表面处理、涂刷和清理；每处不超过1㎡。", "客户提供匹配面漆，可能存在色差；渗水、发霉及严重起皮不在范围内。", 10000},
		{"SIMPLE-CLEAN", "日常保洁套餐", "份", "一名人员提供2小时日常保洁，清洁约定区域可触达表面和地面。", "请提前收好贵重物品。", 10000},
		{"SIMPLE-AREA", "开荒保洁套餐", "份", "提供100㎡以内住宅的约定区域开荒保洁。", "不含装修垃圾清运，需具备水电和进场条件。", 60000},
		{"SIMPLE-INSPECT", "设备检测", "台", "对一台约定类型设备进行基础检测并说明结果。", "", 8000},
	}
}
