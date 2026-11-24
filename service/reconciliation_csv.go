package service

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

type reconciliationCSVColumn struct{ key, en, zh string }

var csvCacheWriteColumns = []reconciliationCSVColumn{
	{"write_standard", "Standard cache write tokens", "普通写入 Token"},
	{"write_5m", "5-minute cache write tokens", "5 分钟写入 Token"},
	{"write_1h", "1-hour cache write tokens", "1 小时写入 Token"},
	{"write_12h", "12-hour cache write tokens", "12 小时写入 Token"},
}

func csvLabels(language, currency string, columns []reconciliationCSVColumn) []string {
	labels := make([]string, len(columns))
	for i, column := range columns {
		labels[i] = csvText(language, column.en, column.zh)
		if currency != "" && strings.HasSuffix(column.key, "_amount") {
			labels[i] += " (" + currency + ")"
		}
	}
	return labels
}

func csvText(language, en, zh string) string {
	if language == "zh" {
		return zh
	}
	return en
}

func csvProjection(columns []reconciliationCSVColumn, values map[string]string) []string {
	record := make([]string, len(columns))
	for i, column := range columns {
		record[i] = values[column.key]
	}
	return record
}

func csvOptionalCount(count *int64) string {
	if count == nil {
		return ""
	}
	return strconv.FormatInt(*count, 10)
}

func csvIdentity(id int64) string {
	if id <= 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

func csvStream(language string, streamed *bool) string {
	if streamed == nil {
		return ""
	}
	if *streamed {
		return csvText(language, "Yes", "是")
	}
	return csvText(language, "No", "否")
}

func csvNotes(notes ...string) string {
	var present []string
	for _, note := range notes {
		if note != "" {
			present = append(present, note)
		}
	}
	return strings.Join(present, " | ")
}

func csvReportCoverage(language string, coverage *model.BillingExportCoverage) string {
	if coverage == nil || (coverage.PublishedDays >= coverage.ExpectedDays && coverage.IncompleteDays == 0 && coverage.EstimatedAt == 0) {
		return ""
	}
	note := fmt.Sprintf(csvText(language, "Partial coverage: %d/%d published days", "部分覆盖，已发布 %d/%d 天"), coverage.PublishedDays, coverage.ExpectedDays)
	if coverage.IncompleteDays > 0 {
		note = csvNotes(note, fmt.Sprintf(csvText(language, "%d incomplete published days", "%d 天覆盖不完整"), coverage.IncompleteDays))
	}
	if coverage.EstimatedAt > 0 {
		note = csvNotes(note, csvText(language, "Provisional as of ", "阶段性数据截至 ")+formatExportTimestamp(coverage.EstimatedAt))
	}
	return note
}

func csvCacheWrites(language string, writes *model.BillingCacheWrites, total int64) ([4]string, string) {
	var cells [4]string
	if writes != nil {
		cells = [4]string{csvOptionalCount(writes.Standard), csvOptionalCount(writes.FiveMinutes), csvOptionalCount(writes.OneHour), csvOptionalCount(writes.TwelveHours)}
		total = writes.Unclassified
	}
	if total == 0 {
		return cells, ""
	}
	return cells, fmt.Sprintf(csvText(language, "Unclassified cache write tokens: %d", "未分档缓存写入 Token %d"), total)
}

func csvCacheWriteValues(values map[string]string, cells [4]string) {
	for i, column := range csvCacheWriteColumns {
		values[column.key] = cells[i]
	}
}

var adminUsageRecordColumns = []reconciliationCSVColumn{
	{key: "record_time", zh: "入账时间"}, {key: "request_id", zh: "请求 ID"}, {key: "event_type", zh: "入账类型"},
	{key: "customer_id", zh: "客户 ID"}, {key: "customer_username", zh: "用户名"}, {key: "token_name", zh: "令牌"}, {key: "token_id", zh: "令牌 ID"},
	{key: "customer_model", zh: "客户模型"}, {key: "is_stream", zh: "流"},
	{key: "net_amount", zh: "净额"}, {key: "original_amount", zh: "原价"}, {key: "quota", zh: "入账额度"},
	{key: "original_quota", zh: "折前额度"}, {key: "counts_toward_customer_bill", zh: "是否计入客户账单"},
	{key: "contract_id", zh: "合同 ID"}, {key: "contract_version", zh: "合同版本"}, {key: "group_ratio", zh: "有效组倍率"},
	{key: "contract_ratio", zh: "合同倍率"}, {key: "final_ratio", zh: "模型最终倍率"}, {key: "billing_mode", zh: "计费方式"},
	{key: "request_count", zh: "计入请求数"}, {key: "input_tokens", zh: "输入 Token"}, {key: "output_tokens", zh: "输出 Token"}, {key: "cache_read_tokens", zh: "缓存读取 Token"},
	csvCacheWriteColumns[0], csvCacheWriteColumns[1], csvCacheWriteColumns[2], csvCacheWriteColumns[3],
	{key: "billable_image_count", zh: "计费张数"}, {key: "billable_seconds", zh: "计费秒数"},
	{key: "billing_line_items_usd", zh: "用量及单价明细（USD，折扣前）"}, {key: "amount_unavailable_reasons", zh: "金额无法计算原因"},
	{key: "contract_expected_amount", zh: "任务合同应收"}, {key: "contract_actual_amount", zh: "任务实际扣款"}, {key: "contract_delta_amount", zh: "任务扣款差额"},
	{key: "field_version", zh: "字段版本"}, {key: "job_id", zh: "导出任务 ID"}, {key: "currency", zh: "币种"},
	{key: "quota_per_unit", zh: "单位金额对应额度"}, {key: "currency_rate", zh: "汇率"},
}

var customerSummaryColumns = []reconciliationCSVColumn{
	{"api_key_name", "API Key", "令牌"}, {"model", "Model", "模型"}, {"billing_mode", "Billing mode", "计费方式"},
	{"net_amount", "Net booked amount", "实际入账净额"}, {"gross_amount", "Gross charges", "累计扣减"}, {"refund_amount", "Refund amount", "累计退回"},
	{"original_amount", "List price", "原价"}, {"discount_amount", "Savings", "优惠"},
	{"requests", "Counted requests", "计入请求数"}, {"billable_calls", "Billable calls", "计费次数"}, {"refunded_calls", "Refunded calls", "退款次数"},
	{"input_tokens", "Input tokens", "输入 Token"}, {"output_tokens", "Output tokens", "输出 Token"}, {"cache_read_tokens", "Cache read tokens", "缓存读取 Token"},
	csvCacheWriteColumns[0], csvCacheWriteColumns[1], csvCacheWriteColumns[2], csvCacheWriteColumns[3],
	{"billable_images", "Billable images", "计费张数"}, {"billable_seconds", "Billable seconds", "计费秒数"}, {"notes", "Reconciliation notes", "对账说明"},
}

func csvMissingOriginal(language, original string) string {
	if original == "" {
		return csvText(language, "Verified list price is incomplete", "原价证据不完整")
	}
	return ""
}

func csvCountNote(language string, count int64, en, zh string) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf(csvText(language, en, zh), count)
}

func csvDurationNote(language string, quantities map[string]decimal.Decimal) string {
	if len(quantities) == 0 {
		return ""
	}
	parts := make([]string, 0, len(quantities))
	for _, key := range slices.Sorted(maps.Keys(quantities)) {
		parts = append(parts, key+" "+quantities[key].String()+csvText(language, " s", " 秒"))
	}
	return csvText(language, "Billable durations ", "计费时长 ") + strings.Join(parts, ", ")
}
