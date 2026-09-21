package model

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func testLogForAmount(quota int) billingReconciliationLog {
	return billingReconciliationLog{Type: LogTypeConsume, TokenId: 0, TokenName: "模型测试", Content: "模型测试", Quota: quota, PromptTokens: 100, CompletionTokens: 50}
}

func TestUpstreamTestAmountForEvidenceTable(t *testing.T) {
	ratio := 1.5
	completion := 2.0
	price := 0.5
	tests := []struct {
		name            string
		quota           int
		prompt          int
		completion      int
		modelRatio      *float64
		completionRatio *float64
		modelPrice      *float64
		expr            bool
		testPricing     *upstreamTestPricingRecord
		groupRatio      *float64
		known           bool
		original        int64
	}{
		{name: "legacy simplified ratio is not native pricing evidence", quota: 300, prompt: 100, completion: 50, modelRatio: &ratio, completionRatio: &completion},
		{name: "ratio mismatch stays pending", quota: 999, prompt: 100, completion: 50, modelRatio: &ratio, completionRatio: &completion},
		{name: "missing completion ratio stays pending", quota: 300, prompt: 100, completion: 50, modelRatio: &ratio},
		{name: "legacy rounded minimum does not prove a priced original", quota: 1, modelRatio: &ratio, completionRatio: &completion},
		{name: "fixed price replays unit quota", quota: 250000, modelPrice: &price, known: true, original: 250000},
		{name: "fixed price mismatch stays pending", quota: 7, modelPrice: &price},
		{name: "legacy zero ratio alone does not prove a free call", quota: 0, modelRatio: new(float64), completionRatio: &completion},
		{name: "no price evidence stays pending even when free", quota: 0},
		{name: "expression without persisted original stays pending", quota: 300, modelRatio: &ratio, completionRatio: &completion, expr: true},
		{name: "explicit settled evidence wins over ratios", quota: 300, testPricing: &upstreamTestPricingRecord{Version: 1, Mode: "tiered_expr", Status: "settled", OriginalQuota: ptrInt64(777)}, known: true, original: 777},
		{name: "unversioned settled amount lacks corrected pricing evidence", testPricing: &upstreamTestPricingRecord{Mode: "ratio", Status: "settled", OriginalQuota: ptrInt64(300)}},
		{name: "versioned explicit zero is known", testPricing: &upstreamTestPricingRecord{Version: 1, Mode: "ratio", Status: "settled", OriginalQuota: ptrInt64(0)}, known: true},
		{name: "unknown pricing mode stays pending", testPricing: &upstreamTestPricingRecord{Version: 1, Mode: "custom", Status: "settled", OriginalQuota: ptrInt64(300)}},
		{name: "explicit estimated evidence stays pending", quota: 300, testPricing: &upstreamTestPricingRecord{Mode: "ratio", Status: "estimated"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log := testLogForAmount(tc.quota)
			log.PromptTokens = tc.prompt
			log.CompletionTokens = tc.completion
			parsed := parsedBillingReconciliationLog{isChannelTest: true, modelRatio: tc.modelRatio, completionRatio: tc.completionRatio, modelPrice: tc.modelPrice, hasExpression: tc.expr, testPricing: tc.testPricing, discountRatio: tc.groupRatio}
			// 原价还原不得依赖测试管理员所属分组：非 1 分组倍率不参与除法。
			group := 3.0
			parsed.discountRatio = &group
			amount := upstreamTestAmountFor(log, parsed)
			assert.Equal(t, tc.known, amount.known)
			assert.Equal(t, !tc.known, amount.pending)
			assert.Equal(t, decimal.NewFromInt(tc.original).String(), amount.original.String())
		})
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestUpstreamTestAmountNotChargedToCustomerProjection(t *testing.T) {
	// 同一条测试记录在客户结算范围内必须保持被排除；上游金额投影入口与
	// 客户范围入口使用同一个判定，不允许第二套身份规则。
	log := testLogForAmount(300)
	assert.True(t, isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content))
}

func TestAccumulateBillingEstimateReasonQualitySplitsReasons(t *testing.T) {
	quality := &BillingReconciliationDataQuality{}
	accumulateBillingEstimateReasonQuality(quality, []string{BillingEstimateAuxiliaryCharge})
	assert.EqualValues(t, 1, quality.AuxiliaryChargeRows)
	assert.EqualValues(t, 0, quality.MissingHistoricalPriceRows, "auxiliary charge is not a missing price")

	accumulateBillingEstimateReasonQuality(quality, []string{BillingEstimateAuxiliaryCharge, BillingEstimateMissingGroup})
	assert.EqualValues(t, 2, quality.AuxiliaryChargeRows)
	assert.EqualValues(t, 1, quality.MissingHistoricalPriceRows, "a mixed row counts once per reason class")

	accumulateBillingEstimateReasonQuality(quality, []string{BillingEstimateMissingContract})
	assert.EqualValues(t, 2, quality.MissingHistoricalPriceRows)
}

func TestHistoricalEstimatedTestProjectionRemainsPendingMoney(t *testing.T) {
	log := testLogForAmount(54)
	log.Other = `{"model_ratio":1.5,"completion_ratio":1,"admin_info":{"local_count_tokens":true}}`
	parsed := parseBillingReconciliationLog(log)
	amount := upstreamTestAmountFor(log, parsed)
	projection := upstreamTestPricingProjection(parsed, amount)
	assert.Equal(t, "estimated", projection.Status)
	assert.Equal(t, "estimated_usage", projection.Reason)
	assert.Nil(t, projection.RecomputedQuota)
	assert.False(t, amount.known, "local estimates must not become confirmed zero amounts")
}
