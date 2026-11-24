package service

import (
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/model"
)

var usageCSVCallColumns = []reconciliationCSVColumn{
	{"calls", "Calls", "调用次数"}, {"success", "Success", "成功"}, {"failure", "Failure", "失败"},
	{"cancelled", "Cancelled", "取消"}, {"other", "Other", "其他终态"},
}

func usageCSVColumns(view, grain string) []reconciliationCSVColumn {
	date := reconciliationCSVColumn{"date", "Date", "日期"}
	if grain == "period" {
		date.en, date.zh = "Period", "账期"
	}
	columns := []reconciliationCSVColumn{date}
	if view == usageAnalyticsExportTypeUpstream {
		columns = append(columns, reconciliationCSVColumn{"channel_id", "Channel ID", "渠道 ID"},
			reconciliationCSVColumn{"channel", "Channel", "渠道"}, reconciliationCSVColumn{"url", "URL grouping", "URL 归组"},
			reconciliationCSVColumn{"model", "Provider model", "上游模型"})
	} else {
		columns = append(columns, reconciliationCSVColumn{"customer", "Customer", "客户"},
			reconciliationCSVColumn{"token", "API Key", "令牌"}, reconciliationCSVColumn{"model", "Model", "模型"})
	}
	columns = append(columns, reconciliationCSVColumn{"mode", "Billing mode", "计费方式"})
	columns = append(columns, usageCSVCallColumns...)
	if view == usageAnalyticsExportTypeUpstream {
		columns = append(columns, reconciliationCSVColumn{"reference_amount", "Reference amount after discount", "折后参考金额"},
			reconciliationCSVColumn{"known_reference_amount", "Known reference subtotal", "已知折后小计"},
			reconciliationCSVColumn{"discount", "Channel discount", "渠道折扣"},
			reconciliationCSVColumn{"input", "Billable input tokens", "计费输入 Token"}, reconciliationCSVColumn{"output", "Billable output tokens", "计费输出 Token"})
	} else {
		columns = append(columns, reconciliationCSVColumn{"net_amount", "Net amount", "净额"}, reconciliationCSVColumn{"gross_amount", "Gross charges", "扣减"},
			reconciliationCSVColumn{"refund_amount", "Refunds", "退回"}, reconciliationCSVColumn{"original_amount", "Estimated list price", "估算原价"},
			reconciliationCSVColumn{"input", "Input tokens", "输入 Token"}, reconciliationCSVColumn{"output", "Output tokens", "输出 Token"})
	}
	columns = append(columns, reconciliationCSVColumn{"cache_read", "Cache read tokens", "缓存读取 Token"})
	columns = append(columns, csvCacheWriteColumns...)
	if view == usageAnalyticsExportTypeUpstream {
		columns = append(columns, reconciliationCSVColumn{"completed", "Completed images", "实际出图张数"},
			reconciliationCSVColumn{"images", "Billable images", "本站计费张数"})
	} else {
		columns = append(columns, reconciliationCSVColumn{"images", "Images", "图片张数"})
	}
	return append(columns, reconciliationCSVColumn{"seconds", "Seconds", "秒数"}, reconciliationCSVColumn{"notes", "Data notes", "数据说明"})
}

func usageCSVMetricValues(metrics model.UsageAnalyticsMetrics, scope customerExportScopeColumns, upstream bool) map[string]string {
	lang := scope.Language
	cacheCells, cacheNote := csvCacheWrites(lang, metrics.CacheWrites, metrics.CacheWriteTokens)
	values := map[string]string{
		"calls": strconv.FormatInt(metrics.TotalCalls, 10), "success": strconv.FormatInt(metrics.SuccessCalls, 10),
		"failure": strconv.FormatInt(metrics.FailureCalls, 10), "cancelled": strconv.FormatInt(metrics.CancelledCalls, 10),
		"other": strconv.FormatInt(metrics.OtherResultCalls, 10),
		"input": strconv.FormatInt(metrics.InputTokens, 10), "output": strconv.FormatInt(metrics.OutputTokens, 10),
		"cache_read":             strconv.FormatInt(metrics.CacheReadTokens, 10),
		"gross_amount":           usageAnalyticsExportAmount(&metrics.GrossQuota, scope),
		"refund_amount":          usageAnalyticsExportAmount(&metrics.RefundQuota, scope),
		"net_amount":             usageAnalyticsExportAmount(&metrics.NetQuota, scope),
		"original_amount":        usageAnalyticsExportAmount(metrics.OriginalQuotaEstimate, scope),
		"reference_amount":       usageAnalyticsExportAmount(metrics.ReferenceAmount, scope),
		"known_reference_amount": usageAnalyticsExportAmount(metrics.KnownReferenceAmount, scope),
		"seconds":                usageAnalyticsFormatSeconds(metrics.Seconds),
	}
	csvCacheWriteValues(values, cacheCells)
	if upstream {
		if !metrics.BillingTokensApplicable {
			values["input"], values["output"], values["cache_read"] = "", "", ""
		}
		if metrics.ImageCountKnown || metrics.ImageCount > 0 {
			values["completed"] = strconv.FormatInt(metrics.ImageCount, 10)
		}
		values["images"] = positiveBillingMeterCSV(metrics.BillableImages)
	} else if metrics.ImageCountKnown || metrics.ImageCount > 0 {
		values["images"] = strconv.FormatInt(metrics.ImageCount, 10)
	}
	if metrics.RowsMissingTokens > 0 {
		for _, field := range []string{"input", "output", "cache_read"} {
			if values[field] == "0" {
				values[field] = ""
			}
		}
	}
	if metrics.RowsMissingMoney > 0 {
		values["gross_amount"], values["refund_amount"] = "", ""
	}
	if metrics.RowsMoneyPending > 0 && metrics.NetQuota == 0 {
		values["net_amount"] = ""
	}
	var unlinkedRefund string
	if !upstream && metrics.UnlinkedRefundQuota != 0 {
		unlinkedRefund = csvText(lang, "Unlinked refund: ", "未关联退款 ") + usageAnalyticsExportAmount(&metrics.UnlinkedRefundQuota, scope)
	}
	var pricingNote string
	if upstream && metrics.CacheWritePricingAdjustedRows > 0 {
		pricingNote = fmt.Sprintf(csvText(lang,
			"Cache columns show observed usage. Pricing classified or weighted cache writes differently in %d calls. Known cache-write pricing operands total %d tokens",
			"缓存分档列为实际用量，%d 次调用的计费包含归档或倍率换算，已知缓存写入计价用量合计 %d Token"),
			metrics.CacheWritePricingAdjustedRows, metrics.CacheWriteTokens)
	}
	values["notes"] = csvNotes(cacheNote, pricingNote, unlinkedRefund, customerExportEstimateReasons(lang, metrics.EstimateReasons),
		csvCountNote(lang, metrics.RowsMissingTokens, "%d records missing usage", "%d 笔缺少用量"),
		csvCountNote(lang, metrics.RowsMissingMoney, "%d records missing money", "%d 笔缺少金额"),
		csvCountNote(lang, metrics.RowsMoneyPending, "%d records awaiting settlement", "%d 笔金额待更新"),
		csvCountNote(lang, metrics.ImageCountMissingRows, "%d records missing images", "%d 笔缺少图片张数"),
		csvCountNote(lang, metrics.SecondsMissingRows, "%d records missing seconds", "%d 笔缺少计费秒数"))
	return values
}
