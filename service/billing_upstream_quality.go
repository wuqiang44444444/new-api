package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

func upstreamExportQualityReasons(quality *model.BillingReconciliationDataQuality, language string) string {
	if quality == nil {
		return ""
	}
	var reasons []string
	for _, reason := range []struct {
		count  int64
		en, zh string
	}{
		{quality.UsageWithoutAmountRows, "%d channel tests are excluded from the amount total; the reasons below partition these tests. Usage is retained.", "%d 条渠道测试未计入金额合计，用量保留；下列原因互不重叠，合计为这些测试的总数。"},
		{quality.TestAmountPendingReasons["estimated_usage"], "%d tests used estimated usage or fees. Their cost is pending and excluded from confirmed totals; this does not mean zero cost.", "其中 %d 条测试的用量或金额为估算值，费用待确认，暂不计入已核算金额；不代表费用为零。"},
		{quality.TestAmountPendingReasons["fixed_price_mismatch"], "%d fixed-price tests have a recorded amount that differs from the saved unit price.", "其中 %d 条按次测试的记录金额与当时保存的单价不符。"},
		{quality.TestAmountPendingReasons["missing_price_fields"], "%d tests did not save the price fields needed to calculate an amount.", "其中 %d 条测试未保存核算金额所需的价格字段。"},
		{quality.TestAmountPendingReasons["invalid_pricing_record"], "%d tests contain invalid or unsupported pricing records.", "其中 %d 条测试的计价记录无效或格式不受支持。"},
		{quality.TestAmountPendingReasons["missing_cache_write"], "%d tests did not save cache-write usage required to recalculate their amount.", "其中 %d 条测试未保存核算所需的缓存写入用量，无法补算这部分费用。"},
		{quality.TestAmountPendingReasons["missing_cache_read"], "%d tests did not save cache-read usage required by their pricing rule.", "其中 %d 条测试未保存计价规则所需的缓存读取用量。"},
		{quality.TestAmountPendingReasons["missing_cache_ttl"], "%d tests lack the cache-write duration breakdown required by their pricing rule.", "其中 %d 条测试缺少按缓存有效期区分的写入用量。"},
		{quality.TestAmountPendingReasons["missing_cache_write_price"], "%d tests have cache writes but lack the historical cache-write price.", "其中 %d 条测试有缓存写入，但未保存当时的写入价格。"},
		{quality.TestAmountPendingReasons["missing_cache_read_price"], "%d tests have cache reads but lack the historical cache-read price.", "其中 %d 条测试有缓存读取，但未保存当时的读取价格。"},
		{quality.TestAmountPendingReasons["missing_usage_semantic"], "%d tests lack the usage breakdown needed to apply their pricing rule.", "其中 %d 条测试缺少计价规则所需的用量分类依据。"},
		{quality.TestAmountPendingReasons["missing_multimodal_usage"], "%d tests lack the image or audio usage needed by their pricing rule.", "其中 %d 条测试缺少计价规则所需的图片或音频用量。"},
		{quality.TestAmountPendingReasons["missing_expression_context"], "%d tests lack the original request parameters or pricing time required by their rule.", "其中 %d 条测试未保存计价规则所需的请求参数或计价时刻。"},
		{quality.TestAmountPendingReasons["missing_group_ratio"], "%d expression tests lack the group multiplier needed to verify their recorded amount.", "其中 %d 条表达式测试缺少核验记录金额所需的分组倍率。"},
		{quality.TestAmountPendingReasons["recorded_amount_mismatch"], "%d tests have a recorded amount that disagrees with the frozen rule and usage.", "其中 %d 条测试的记录金额与保存的规则、用量复算结果不一致。"},
		{quality.TestAmountPendingReasons["missing_tool_price"], "%d tests include tool fees without a complete historical fee breakdown.", "其中 %d 条测试含工具附加费，但未保存完整费用明细。"},
		{quality.CacheReadUnavailableRequests - quality.CacheReadUnreportedRequests - quality.LegacyTestCacheReadRows, "%d billing records lack cache read details.", "%d 条账单记录缺少缓存读取明细。"},
		{quality.LegacyTestCacheReadRows, "%d of the unpriced tests above also lack cache read details; these are the same records.", "其中 %d 条未计价测试还缺少缓存读取明细，已包含在上述测试记录中。"},
		{quality.CacheReadUnreportedRequests, "%d responses did not provide a usable cache read meter; this does not imply a failed request.", "%d 条响应未提供可用的缓存读取计量，不代表请求失败。"},
		{quality.SecondsUnavailableRows - quality.SecondsValueMissingRows - quality.SecondsTaskLinkMissingRows, "%d per-second records lack the recorded billing unit.", "%d 条按秒账单未保存计量单位。"},
		{quality.SecondsTaskLinkMissingRows, "%d historical video records have no task link or recorded billing seconds.", "%d 条历史视频记录缺少任务关联，日志中也未保存计费秒数。"},
		{quality.SecondsValueMissingRows - quality.SecondsTaskLinkMissingRows, "%d per-second records lack the billable duration used at the time; their amounts cannot be recalculated from usage.", "%d 条按秒账单未保存当时的计费秒数，无法根据用量复算已记录的金额。"},
		{quality.AuxiliaryChargeRows, "%d records include tool surcharges; their official price cannot be fully restored yet.", "%d 条记录包含工具附加费，原价暂无法完整还原。"},
		{quality.InputTokensUnavailableRequests, "%d records have no confirmed total input usage.", "%d 条记录缺少可确认的总输入用量。"},
		{quality.CacheWriteUnavailableRequests - quality.CacheWriteUnreportedRequests - quality.LegacyTestCacheWriteRows, "%d billing records lack cache write details.", "%d 条账单记录缺少缓存写入明细。"},
		{quality.LegacyTestCacheWriteRows, "%d of the unpriced tests above also lack cache write details; these are the same records.", "其中 %d 条未计价测试还缺少缓存写入明细，已包含在上述测试记录中。"},
		{quality.CacheWriteUnreportedRequests, "%d responses did not provide a usable cache write meter; this does not imply a failed request.", "%d 条响应未提供可用的缓存写入计量，不代表请求失败。"},
		{quality.UnavailableRequests, "%d records are missing readable billing metadata.", "%d 条记录缺少可读取的计费信息。"},
		{quality.UnknownBillingModeRequests, "%d records do not contain a frozen billing mode.", "%d 条记录缺少发生时的计费方式。"},
		{quality.ProviderModelFallbackRows, "%d records do not contain the Provider model identity; the customer model is shown instead.", "%d 条记录缺少上游模型身份，暂显示客户模型。"},
		{quality.MissingHistoricalPriceRows, "%d records are missing historical price or discount snapshots.", "%d 条记录缺少历史价格或折扣快照。"},
	} {
		if reason.count <= 0 {
			continue
		}
		label := reason.en
		if language == "zh" {
			label = reason.zh
		}
		reasons = append(reasons, fmt.Sprintf(label, reason.count))
	}
	return strings.Join(reasons, "; ")
}
