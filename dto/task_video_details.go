package dto

// TaskVideoDetails 是任务详情弹窗的视频参数投影，只由已保存事实构建：
// 不从当前渠道、模型配置或价格重新解释历史任务，不为展示触发上游查询。
// 每组参数用指针区分“未记录”（nil）与显式值；布尔参数区分显式 false 与缺失。
type TaskVideoDetails struct {
	// Request 是客户请求快照（创建时冻结的统一北向参数）。
	Request *TaskVideoRequestParams `json:"request,omitempty"`
	// Billing 是计费采用参数（冻结计费探针归一化后的取值，含默认值与
	// 智能时长上限换算），不能冒充客户显式传值或南向发送内容。
	Billing *TaskVideoBillingParams `json:"billing,omitempty"`
	// Settlement 是结算事实（关联既有冻结账务记录，不重建账本）。
	Settlement *TaskVideoSettlement `json:"settlement,omitempty"`
}

type TaskVideoRequestParams struct {
	Seconds       *TaskVideoTextParam `json:"seconds,omitempty"`
	Resolution    *TaskVideoTextParam `json:"resolution,omitempty"`
	Ratio         *TaskVideoTextParam `json:"ratio,omitempty"`
	GenerateAudio *TaskVideoBoolParam `json:"generate_audio,omitempty"`
	ServiceTier   *TaskVideoTextParam `json:"service_tier,omitempty"`
}

type TaskVideoBillingParams struct {
	DurationSeconds *TaskVideoTextParam `json:"duration_seconds,omitempty"`
	Resolution      *TaskVideoTextParam `json:"resolution,omitempty"`
	GenerateAudio   *TaskVideoBoolParam `json:"generate_audio,omitempty"`
}

type TaskVideoSettlement struct {
	Quota               int                `json:"quota"`
	BillingState        string             `json:"billing_state,omitempty"`
	OtherRatios         map[string]float64 `json:"other_ratios,omitempty"`
	ActualUsage         map[string]int     `json:"actual_usage,omitempty"`
	ActualUsageReported bool               `json:"actual_usage_reported,omitempty"`
}

// TaskVideoTextParam 携带一个字符串/数值参数；指针存在即表示有可信记录。
type TaskVideoTextParam struct {
	Value string `json:"value"`
}

// TaskVideoBoolParam 携带一个布尔参数；显式 false 保留为 false，
// 与“未记录”（上层以 nil 指针表示）严格区分。
type TaskVideoBoolParam struct {
	Value bool `json:"value"`
}
