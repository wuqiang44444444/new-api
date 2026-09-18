package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// The statement uses the same safe facts, money conversion and streaming
// writer as usage exports, with a customer-facing column projection.
var customerStatementDetailColumns = []struct{ key, en, zh string }{
	{"record_time", "Record time", "入账时间"},
	{"request_id", "Request ID", "请求 ID"},
	{"token_name", "API Key", "API 密钥"},
	{"token_id", "API Key ID", "API 密钥 ID"},
	{"customer_model", "Model", "模型"},
	{"event_type", "Billing event", "入账类型"},
	{"currency", "Currency", "币种"},
	{"net_amount", "Net amount (refunds negative)", "净额（退款为负）"},
	{"original_amount_estimated", "Estimated list price", "估算原价"},
	{"discount_amount_estimated", "Estimated savings", "估算优惠"},
	{"estimate_reasons", "Estimate reasons", "估算未完成原因"},
	{"billing_mode", "Billing mode", "计费方式"},
	{"request_count", "Counted requests", "计入请求数"},
	{"input_tokens", "Input tokens", "输入 Token"},
	{"output_tokens", "Output tokens", "输出 Token"},
	{"cache_read_tokens", "Cache read tokens", "缓存读取 Token"},
	{"cache_write_tokens", "Cache write tokens", "缓存写入 Token"},
	{"input_tokens_quality", "Input usage status", "输入用量状态"},
	{"group_name", "Billing group", "计费组"},
	{"group_ratio_source", "Group ratio source", "组倍率来源"},
	{"group_ratio", "Group factor", "分组倍率"},
	{"contract_applicable", "Contract status", "合同状态"},
	{"contract_name", "Contract", "合同名称"},
	{"contract_ratio", "Contract factor", "合同倍率"},
	{"final_ratio", "Final factor", "最终倍率"},
	{"matched_tier", "Pricing tier", "计价阶梯"},
	{"billing_line_items_usd", "Usage and price breakdown (USD, before discounts)", "用量及单价明细（USD，折扣前）"},
	{"billing_explanation_status", "Price breakdown status", "单价明细状态"},
	{"quality_status", "Data quality", "数据质量"},
	{"period_start", "Period start", "账期开始"},
	{"period_end", "Period end", "账期结束"},
	{"timezone", "Timezone", "时区"},
}

func (w *customerExportCsvWriter) useStatementDetails(language string) {
	w.statementLanguage = &language
	w.prefix = "statement-details"
	w.header = make([]string, len(customerStatementDetailColumns))
	for i, column := range customerStatementDetailColumns {
		w.header[i] = column.en
		if language == "zh" {
			w.header[i] = column.zh
		}
	}
}

func customerStatementDetailRecord(language string, source []string) []string {
	values := make(map[string]string, len(source))
	for i, key := range customerExportCsvHeader {
		values[key] = source[i]
	}
	record := make([]string, len(customerStatementDetailColumns))
	for i, column := range customerStatementDetailColumns {
		value := values[column.key]
		if column.key == "billing_line_items_usd" {
			value = customerStatementPriceBreakdown(language, value)
		} else {
			value = customerStatementExportValue(language, column.key, value)
		}
		record[i] = value
	}
	return record
}

var customerStatementExportValues = map[string]map[string][2]string{
	"billing_mode":               {"token": {"Per token", "按 Token"}, "per_call": {"Per call", "按次"}, "per_second": {"Per second", "按秒"}, "unknown": {"Not recorded", "未记录"}},
	"event_type":                 {"consume": {"Charge", "扣款"}, "refund": {"Refund", "退款"}},
	"contract_applicable":        {"yes": {"Applied", "已应用"}, "no": {"No contract discount", "无合同优惠"}, "unrecorded": {"Not recorded", "未记录"}, "unknown": {"Incomplete contract records", "合同记录不完整"}},
	"group_ratio_source":         {"group": {"Group", "分组倍率"}, "user_exclusive": {"Customer-specific group factor", "客户专属组倍率"}, "unknown": {"Not recorded", "未记录"}},
	"quality_status":             {"exact": {"Complete", "完整"}, "estimate": {"Estimated", "估算"}, "unrecorded": {"Not recorded", "未记录"}, "not_billing_event": {"Not a billing event", "非计费事件"}, "complete": {"Complete", "完整"}, "partial": {"Incomplete records", "记录不完整"}},
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
