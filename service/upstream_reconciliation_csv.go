package service

var upstreamDetailColumns = []reconciliationCSVColumn{
	{"time", "Time", "时间"}, {"channel_id", "Channel ID", "渠道 ID"}, {"upstream_request", "Upstream request ID", "上游请求 ID"},
	{"request", "Our request ID", "我方请求 ID"}, {"event", "Row kind", "记录类型"}, {"channel", "Channel", "渠道"},
	{"name", "Upstream name", "上游名称"}, {"url", "URL grouping", "URL 归组"}, {"provider_model", "Provider model", "上游模型"},
	{"customer_model", "Customer model", "客户模型"}, {"stream", "Stream", "流"},
	{"reference_amount", "Reference amount after discount", "折后参考金额"}, {"original_amount", "Local list price", "本地官方原价"},
	{"discount", "Channel discount", "渠道折扣"}, {"version", "Discount version", "折扣版本"}, {"status", "Original amount status", "核算状态"},
	{"mode", "Billing mode", "计费方式"}, {"input", "Input tokens", "输入 Token"}, {"output", "Output tokens", "输出 Token"},
	{"cache_read", "Cache read tokens", "缓存读取 Token"}, csvCacheWriteColumns[0], csvCacheWriteColumns[1], csvCacheWriteColumns[2], csvCacheWriteColumns[3],
	{"seconds", "Billable seconds", "计费秒数"}, {"images", "Billable images", "计费张数"}, {"completed", "Completed images", "实际出图张数"},
	{"row_id", "Record ID", "记录 ID"}, {"task", "Platform task ID", "平台任务 ID"}, {"upstream_task", "Upstream task ID", "上游任务 ID"},
	{"notes", "Reconciliation notes", "对账说明"},
}

var upstreamSummaryColumns = []reconciliationCSVColumn{
	{"channel_id", "Channel ID", "渠道 ID"}, {"channel", "Channel", "渠道"}, {"name", "Upstream name", "上游名称"},
	{"url", "URL grouping", "URL 归组"}, {"provider_model", "Provider model", "上游模型"}, {"mode", "Billing mode", "计费方式"},
	{"reference_amount", "Reference amount after discount", "折后参考金额"}, {"original_amount", "Local list price", "本地官方原价"},
	{"discount", "Channel discount", "渠道折扣"}, {"version", "Discount version", "折扣版本"},
	{"known_reference_amount", "Known reference subtotal", "已知折后小计"}, {"gaps", "Records pending reconciliation", "待核算记录数"},
	{"requests", "Requests", "请求数"}, {"calls", "Billable calls", "计费调用数"},
	{"input", "Input tokens", "输入 Token"}, {"output", "Output tokens", "输出 Token"}, {"cache_read", "Cache read tokens", "缓存读取 Token"},
	csvCacheWriteColumns[0], csvCacheWriteColumns[1], csvCacheWriteColumns[2], csvCacheWriteColumns[3],
	{"seconds", "Billable seconds", "计费秒数"}, {"images", "Billable images", "本站计费张数"}, {"completed", "Completed images", "实际出图张数"},
	{"test_images", "Channel test billable images", "渠道测试计价张数"}, {"notes", "Reconciliation notes", "对账说明"},
}
