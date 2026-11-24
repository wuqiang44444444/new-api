package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// The statement uses the same safe facts, money conversion and streaming
// writer as usage exports, with a customer-facing column projection.
var customerStatementDetailColumns = []reconciliationCSVColumn{
	{"record_time", "Record time", "入账时间"},
	{"request_id", "Request ID", "请求 ID"},
	{"event_type", "Billing event", "入账类型"},
	{"token_name", "API Key", "令牌"},
	{"customer_model", "Model", "模型"},
	{"is_stream", "Stream", "流"},
	{"net_amount", "Net booked amount (refunds negative)", "实际入账净额（退款为负）"},
	{"original_amount", "List price", "原价"},
	{"contract_id", "Contract ID", "合同 ID"},
	{"contract_version", "Contract version", "合同版本"},
	{"group_ratio", "Effective group factor", "有效组倍率"},
	{"contract_ratio", "Contract factor", "合同倍率"},
	{"final_ratio", "Final model factor", "模型最终倍率"},
	{"billing_mode", "Billing mode", "计费方式"},
	{"request_count", "Counted requests", "计入请求数"},
	{"input_tokens", "Input tokens", "输入 Token"},
	{"output_tokens", "Output tokens", "输出 Token"},
	{"cache_read_tokens", "Cache read tokens", "缓存读取 Token"},
	csvCacheWriteColumns[0], csvCacheWriteColumns[1], csvCacheWriteColumns[2], csvCacheWriteColumns[3],
	{"billable_image_count", "Billable images", "计费张数"},
	{"billable_seconds", "Billable seconds", "计费秒数"},
	{"notes", "Billing notes", "计费说明"},
}

func (w *customerExportCsvWriter) useStatementDetails(language, currency string) {
	w.statementLanguage = &language
	w.prefix = "statement-details"
	w.header = csvLabels(language, currency, customerStatementDetailColumns)
}

func customerStatementDetailRecord(language string, row model.CustomerExportRow, scope customerExportScopeColumns, admin bool) []string {
	net := row.Quota
	if row.EventType == "refund" {
		net = -net
	}
	values := map[string]string{
		"record_time": formatExportTimestamp(row.RecordTime), "request_id": exportCsvCellGuard(row.RequestId),
		"token_name": exportCsvCellGuard(row.TokenName), "customer_model": exportCsvCellGuard(row.CustomerModel),
		"net_amount": exportCurrencyAmount(strconv.FormatInt(net, 10), scope), "original_amount": exportCurrencyAmount(row.RecordedOriginal, scope),
		"group_ratio": exportRatio(row.GroupRatio), "contract_ratio": exportRatio(row.ContractRatio), "final_ratio": exportRatio(row.FinalRatio),
		"request_count": strconv.FormatInt(row.RequestCount, 10), "input_tokens": strconv.FormatInt(row.InputTokens, 10),
		"output_tokens": strconv.FormatInt(row.OutputTokens, 10), "cache_read_tokens": strconv.FormatInt(row.CacheReadTokens, 10),
		"billable_image_count": csvOptionalCount(row.BillableImageCount),
	}
	if row.InputTokensUnavailable {
		values["input_tokens"] = ""
	}
	values["is_stream"] = csvStream(language, row.IsStream)
	values["contract_id"] = csvIdentity(row.ContractID)
	values["contract_version"] = csvIdentity(row.ContractVersion)
	if row.ContractApplicable == "no" {
		values["contract_ratio"] = "1"
	}
	if row.BillableSeconds != nil {
		values["billable_seconds"] = row.BillableSeconds.String()
	}
	cacheCells, cacheNote := csvCacheWrites(language, row.CacheWrites, row.CacheWriteTokens)
	csvCacheWriteValues(values, cacheCells)
	if row.CacheReadUnknown {
		values["cache_read_tokens"] = ""
	}
	if row.BillingMode == model.BillingReconciliationModePerImage {
		values["input_tokens"], values["output_tokens"], values["cache_read_tokens"] = "", "", ""
		csvCacheWriteValues(values, [4]string{})
		cacheNote = ""
	}
	values["event_type"] = customerStatementExportValue(language, "event_type", row.EventType)
	values["billing_mode"] = customerStatementExportValue(language, "billing_mode", row.BillingMode)
	var expected, actual, delta string
	if settlement := row.ContractSettlement; settlement != nil {
		expected = exportCurrencyAmount(strconv.FormatInt(settlement.ExpectedQuota, 10), scope)
		actual = exportCurrencyAmount(strconv.FormatInt(settlement.ActualQuota, 10), scope)
		delta = exportCurrencyAmount(strconv.FormatInt(settlement.DeltaQuota, 10), scope)
	}
	reasons := customerExportEstimateReasons(language, row.EstimateReasons)
	durations := csvDurationNote(language, row.BillableDurations)
	if admin {
		values["token_id"] = strconv.Itoa(row.TokenId)
		values["customer_id"] = strconv.Itoa(scope.CustomerId)
		values["customer_username"] = exportCsvCellGuard(scope.CustomerName)
		values["quota"] = strconv.FormatInt(row.Quota, 10)
		values["original_quota"] = row.RecordedOriginal
		values["billing_line_items_usd"] = csvNotes(customerStatementPriceBreakdown(language, row.BillingLineItems), durations)
		values["counts_toward_customer_bill"] = csvStream(language, &row.CountsTowardBill)
		values["amount_unavailable_reasons"] = csvNotes(reasons, cacheNote)
		values["contract_expected_amount"], values["contract_actual_amount"], values["contract_delta_amount"] = expected, actual, delta
		values["field_version"] = strconv.Itoa(customerExportFieldVersion)
		values["job_id"] = scope.JobID
		values["currency"] = scope.Currency
		values["quota_per_unit"] = strconv.FormatFloat(scope.QuotaPerUnit, 'f', -1, 64)
		values["currency_rate"] = strconv.FormatFloat(scope.CurrencyRate, 'f', -1, 64)
		return csvProjection(adminUsageRecordColumns, values)
	}
	var settlement, imageEvidence string
	if row.ContractSettlement != nil {
		settlement = fmt.Sprintf(csvText(language, "Contract charge %s, actual charge %s, difference %s", "任务合同应收 %s，实扣 %s，差额 %s"),
			expected, actual, delta)
	}
	if count := row.DerivedOutputImageTokens; count != nil {
		imageEvidence = csvText(language, "Derived output image tokens ", "推导的输出图片 Token ") + strconv.Itoa(*count)
	}
	values["notes"] = csvNotes(cacheNote, reasons, settlement, durations,
		customerStatementExportValue(language, "original_source", row.OriginalSource), imageEvidence)
	return csvProjection(customerStatementDetailColumns, values)
}

var customerStatementExportValues = map[string]map[string][2]string{
	"billing_mode":               {"token": {"Per token", "按 Token"}, "per_call": {"Per call", "按次"}, model.BillingReconciliationModePerImage: {"Per image", "按张"}, "per_second": {"Per second", "按秒"}, model.BillingReconciliationModeDurationAndCount: {"Duration and quantity", "按时长和数量"}, "unknown": {"Not recorded", "未记录"}},
	"event_type":                 {"consume": {"Charge", "扣款"}, "refund": {"Refund", "退款"}},
	"original_source":            {"replayed": {"Verified replay", "核验复算"}, "unique_settlement_replay": {"Unique solution verified against the actual charge", "以实扣核验的唯一解"}},
	"contract_applicable":        {"yes": {"Applied", "已应用"}, "no": {"No contract discount", "无合同优惠"}, "unrecorded": {"Not recorded", "未记录"}, "unknown": {"Incomplete contract records", "合同记录不完整"}},
	"group_ratio_source":         {"group": {"Group", "分组倍率"}, "user_exclusive": {"Customer-specific group factor", "客户专属组倍率"}, "unknown": {"Not recorded", "未记录"}},
	"quality_status":             {"exact": {"Complete", "完整"}, "unrecorded": {"Not recorded", "未记录"}, "not_billing_event": {"Not a billing event", "非计费事件"}, "complete": {"Complete", "完整"}, "partial": {"Incomplete records", "记录不完整"}},
	"input_tokens_quality":       {"known": {"Recorded", "已记录"}, "unknown": {"Not recorded", "未记录"}},
	"billing_explanation_status": {"available": {"Available", "已记录"}, "unavailable": {"Insufficient historical price records", "历史单价记录不足"}, "not_applicable": {"Not applicable", "不适用"}},
	"line_label":                 {"Input": {"Input", "输入"}, "Output": {"Output", "输出"}, "Cache read": {"Cache read", "缓存读取"}, "Cache write": {"Cache write", "缓存写入"}},
}

func customerStatementExportValue(language, column, value string) string {
	if labels, ok := customerStatementExportValues[column][value]; ok {
		if language == "zh" {
			return labels[1]
		}
		return labels[0]
	}
	return value
}

// Each event remains one row even when it has multiple metered line items.
// Unit prices retain their recorded USD basis and price quantity (e.g. 1M tokens).
func customerStatementPriceBreakdown(language, raw string) string {
	var lines []struct {
		customerBillingLine
		PriceQuantity int `json:"unit_price_quantity"`
	}
	if common.UnmarshalJsonStr(raw, &lines) != nil {
		return ""
	}
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		priceBasis := line.Unit
		if line.PriceQuantity > 0 {
			priceBasis = strconv.Itoa(line.PriceQuantity) + " " + line.Unit
		}
		parts = append(parts, fmt.Sprintf("%s: %g %s; %g USD / %s; %g USD",
			customerStatementExportValue(language, "line_label", line.Label), line.Quantity, line.Unit, line.UnitPrice, priceBasis, line.Subtotal))
	}
	return exportCsvCellGuard(strings.Join(parts, " | "))
}
