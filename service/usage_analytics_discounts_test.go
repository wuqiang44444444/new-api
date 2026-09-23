package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUsageDiscountExportExplainsHistoricalCombination(t *testing.T) {
	group, contract := 0.8, 0.5
	original, savings := int64(100), int64(60)
	row := model.UsageCustomerModelRow{DiscountCombinations: []model.BillingDiscountCombination{{GroupName: "historical-group", GroupRatio: &group, ContractApplicable: "yes", ContractName: "historical-contract", ContractVersion: 2, ContractRatio: &contract, OriginalQuota: &original, DiscountQuota: &savings, Usage: model.BillingReconciliationUsage{NetQuota: 40, RefundQuota: 10}}}}
	scope := customerExportScopeColumns{Currency: "USD", QuotaPerUnit: 100, CurrencyRate: 1}
	text := usageCustomerDiscountExport(row, "en", scope)
	assert.Contains(t, text, "Group: 0.8 (historical-group)")
	assert.Contains(t, text, "contract: 0.5 (historical-contract, v2)")
	assert.Contains(t, text, "final factor: 0.4")
	assert.Contains(t, text, "estimated original: 1")
	assert.Contains(t, text, "estimated savings: 0.6")
	assert.Contains(t, usageCustomerDiscountExport(row, "zh", scope), "综合倍率: 0.4")
	row.DiscountCombinations[0].ContractApplicable = "unrecorded"
	assert.Contains(t, usageCustomerDiscountExport(row, "en", scope), "final factor: Not recorded")
	row.Total.RowsMissingMoney = 1
	assert.Equal(t, "Awaiting complete settlement records", usageCustomerDiscountExport(row, "en", scope))
}
