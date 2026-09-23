package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// 只在模型周期合计行输出本周期组合，不把周期折扣重复标成每日实际折扣。
func usageCustomerDiscountExport(row model.UsageCustomerModelRow, language string, scope customerExportScopeColumns) string {
	unknown, pending, none := "Not recorded", "Awaiting complete settlement records", "No contract discount applied"
	pattern := "Group: %s (%s); contract: %s (%s); final factor: %s; estimated original: %s; estimated savings: %s; net: %s; refund: %s"
	if language == "zh" {
		unknown, pending, none = "未记录", "待完整结算记录", "未适用合同折扣"
		pattern = "分组: %s (%s); 合同: %s (%s); 综合倍率: %s; 估算原价: %s; 估算优惠: %s; 净额: %s; 退款: %s"
	}
	if row.Total.RowsMissingMoney > 0 || row.Total.RowsMoneyPending > 0 {
		return pending
	}
	if len(row.DiscountCombinations) == 0 {
		return unknown
	}
	parts := make([]string, 0, len(row.DiscountCombinations))
	for _, combo := range row.DiscountCombinations {
		group, contract, final := unknown, unknown, unknown
		invalid := combo.Other
		for _, reason := range combo.EstimateReasons {
			if reason == "invalid_facts" {
				invalid = true
			}
		}
		if !invalid && combo.GroupRatio != nil {
			group = decimal.NewFromFloat(*combo.GroupRatio).String()
		}
		if !invalid && combo.ContractApplicable == "no" {
			contract = none
		}
		if !invalid && combo.ContractApplicable == "yes" && combo.ContractRatio != nil {
			contract = decimal.NewFromFloat(*combo.ContractRatio).String()
		}
		if !invalid && combo.GroupRatio != nil {
			if combo.ContractApplicable == "no" {
				final = group
			}
			if combo.ContractApplicable == "yes" && combo.ContractRatio != nil {
				final = decimal.NewFromFloat(*combo.GroupRatio).Mul(decimal.NewFromFloat(*combo.ContractRatio)).String()
			}
		}
		original, savings := unknown, unknown
		if combo.OriginalQuota != nil {
			original = usageAnalyticsExportAmount(combo.OriginalQuota, scope)
		}
		if combo.DiscountQuota != nil {
			savings = usageAnalyticsExportAmount(combo.DiscountQuota, scope)
		}
		contractName := combo.ContractName
		if combo.ContractApplicable == "yes" {
			if contractName == "" {
				contractName = unknown
			}
			if combo.ContractVersion > 0 {
				contractName += fmt.Sprintf(", v%d", combo.ContractVersion)
			}
		}
		parts = append(parts, fmt.Sprintf(pattern, group, combo.GroupName, contract, contractName, final, original, savings, usageAnalyticsExportAmount(&combo.Usage.NetQuota, scope), usageAnalyticsExportAmount(&combo.Usage.RefundQuota, scope)))
	}
	return exportCsvCellGuard(strings.Join(parts, "\n"))
}
