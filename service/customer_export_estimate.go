package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// CSVs carry the same customer-facing explanations as the page, using the
// language frozen for the export. Reason codes remain in the API projection.
func customerExportEstimateReasons(language string, reasons []string) string {
	labels := map[string][2]string{
		model.BillingEstimateMissingContract:  {"Cannot estimate: historical contract status not recorded", "无法估算：历史合同状态未记录"},
		model.BillingEstimateMissingGroup:     {"Cannot estimate: historical group ratio not recorded", "无法估算：历史分组倍率未记录"},
		model.BillingEstimateInvalidFacts:     {"Estimate failed: invalid billing records", "估算失败：计费记录异常"},
		model.BillingEstimateAuxiliaryCharge:  {"Cannot estimate: historical surcharge conversion is unverified", "无法估算：附加费历史换算依据未核实"},
		model.BillingEstimateAmountOutOfRange: {"Estimate failed: amount out of range", "估算失败：金额超出范围"},
		model.BillingEstimateCombinationLimit: {"Cannot estimate: combined row details unavailable", "无法估算：合并行缺少折扣明细"},
	}
	index := 0
	if normalizeCustomerExportLanguage(language) == "zh" {
		index = 1
	}
	result := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		label, ok := labels[reason]
		if !ok {
			label = [2]string{"Cannot estimate: reason not recorded in this historical statement", "无法估算：历史账单未记录原因"}
		}
		result = append(result, label[index])
	}
	return strings.Join(result, "; ")
}
