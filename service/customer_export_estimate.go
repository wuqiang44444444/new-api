package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// CSVs carry the same customer-facing explanations as the page, using the
// language frozen for the export. Reason codes remain in the API projection.
func customerExportEstimateReasons(language string, reasons []string) string {
	labels := map[string][2]string{
		"original_quota_not_recorded":             {"Verified list price is unavailable", "缺少可核实的原价"},
		model.BillingOriginalReplayMissingContext: {"Historical pricing inputs are incomplete", "历史计价依据不完整"},
		model.BillingOriginalReplayMissingUsage:   {"Historical metered usage is incomplete", "历史计费用量不完整"},
		model.BillingOriginalReplayMismatch:       {"Recalculated charge differs from settlement", "复算金额与实际入账不一致"},
		model.BillingOriginalReplayUnverified:     {"Original amount could not be verified for this event", "此账务事件的原价无法核实"},
		model.BillingEstimateMissingContract:      {"Cannot calculate: historical contract status not recorded", "无法计算：历史合同状态未记录"},
		model.BillingEstimateMissingGroup:         {"Cannot calculate: historical group ratio not recorded", "无法计算：历史分组倍率未记录"},
		model.BillingEstimateInvalidFacts:         {"Cannot calculate: invalid billing records", "无法计算：计费记录异常"},
		model.BillingEstimateAuxiliaryCharge:      {"Cannot calculate: historical surcharge conversion is unverified", "无法计算：附加费历史换算依据未核实"},
		model.BillingEstimateAmountOutOfRange:     {"Cannot calculate: amount out of range", "无法计算：金额超出范围"},
		model.BillingEstimateTestAmountPending:    {"Channel test amount pending", "渠道测试金额待核算"},
		model.BillingEstimateCombinationLimit:     {"Cannot calculate: combined row details unavailable", "无法计算：合并行缺少折扣明细"},
	}
	index := 0
	if normalizeCustomerExportLanguage(language) == "zh" {
		index = 1
	}
	result := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		label, ok := labels[reason]
		if !ok {
			label = [2]string{"Cannot calculate: reason not recorded in this historical statement", "无法计算：历史账单未记录原因"}
		}
		result = append(result, label[index])
	}
	return strings.Join(result, "; ")
}
