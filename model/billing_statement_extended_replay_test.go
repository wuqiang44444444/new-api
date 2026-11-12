package model

import (
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerOriginalReplayUsesFrozenEvaluationFacts(t *testing.T) {
	for _, tc := range []struct {
		name, expression, fields string
		booked, original         int
	}{
		{
			name:       "logged unique tier selects the historical time branch",
			expression: `hour("UTC") > 6 ? tier("peak", p * 3) : tier("off", p * 2)`,
			fields:     `"matched_tier":"peak","billing_unit":"token"`,
			booked:     113, original: 150,
		},
		{
			name:       "legacy token formula proves omitted unit",
			expression: `tier("base", p * 2)`,
			fields:     `"matched_tier":"base"`,
			booked:     75, original: 100,
		},
		{
			name:       "evaluated per-image price and quantity freeze the result",
			expression: `(param("quality") == "high" ? tier("base", fixed(0.03)) : tier("base", fixed(0.01))) * image_count`,
			fields:     `"matched_tier":"base","billing_unit":"request","fixed_price":0.03,"image_count":2`,
			booked:     22500, original: 30000,
		},
		{
			name:       "tool fee has its own group factor and no contract discount",
			expression: `tier("base", p * 2)`,
			fields:     `"matched_tier":"base","billing_unit":"token","tool_surcharges":[{"count":2,"price":1}]`,
			booked:     1075, original: 1100,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{Type: LogTypeConsume, UserId: 1, Quota: tc.booked, PromptTokens: 100,
				Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":1,"contract_discount":0.75,"usage_semantic":"openai",%s}`,
					base64.StdEncoding.EncodeToString([]byte(tc.expression)), tc.fields)}
			parsed := parseBillingReconciliationLog(log)
			amount, reason := replayCustomerOriginal(log, parsed)
			require.Empty(t, reason)
			assert.EqualValues(t, tc.original, amount)

			log.Quota++
			_, reason = replayCustomerOriginal(log, parsed)
			assert.Equal(t, BillingOriginalReplayMismatch, reason)
		})
	}
}

func TestCustomerOriginalReplayDoesNotInferAmbiguousTimeTier(t *testing.T) {
	expression := base64.StdEncoding.EncodeToString([]byte(`hour("UTC") > 6 ? tier("same", p * 3) : tier("same", p * 2)`))
	log := billingReconciliationLog{Type: LogTypeConsume, UserId: 1, Quota: 75, PromptTokens: 100,
		Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":1,"contract_discount":0.75,"usage_semantic":"openai","matched_tier":"same","billing_unit":"token"}`, expression)}
	_, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Equal(t, BillingOriginalReplayMissingContext, reason)
}

func TestCustomerOriginalReplayNativeImageUsesFrozenRatios(t *testing.T) {
	log := billingReconciliationLog{Type: LogTypeConsume, UserId: 1, Quota: 105, PromptTokens: 100,
		Other: `{"request_path":"/v1/images/generations","usage_semantic":"openai","model_price":-1,"model_ratio":1,"completion_ratio":2,"cache_ratio":0.5,"cache_tokens":0,"image":true,"image_output":40,"image_ratio":2,"group_ratio":1,"contract_discount":0.75}`}
	amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	require.Empty(t, reason)
	assert.EqualValues(t, 140, amount)
}

func TestTaskStatementOriginalReplayVerifiesLegacyConversionAndDelivery(t *testing.T) {
	calculation := billingexpr.NewCalculation()
	calculation.Add("quota_conversion", "quota", 150, 300, 500000, "1e+06")
	calculation.Add("group_ratio", "quota", 150, 150, 1)
	calculation.Add("contract_ratio", "quota", 112.5, 150, 0.75)
	calculation.Add("round", "quota", 113, 112.5)
	calculation.Finish(113)
	task := &Task{PrivateData: TaskPrivateData{
		BillingContext: &TaskBillingContext{ContractFact: &types.ContractBillingFact{RatioUnits: 75_000_000}},
		AsyncBilling: &TaskAsyncBillingContext{Calculation: calculation, CalculationSource: "settlement",
			TieredSnapshot: &billingexpr.BillingSnapshot{QuotaPerUnit: 500000, GroupRatio: 1}},
	}}
	delivery := TaskBillingDelivery{Event: "complete", AfterQuota: 113}
	original := TaskStatementOriginalQuota(task, delivery, LogTypeConsume)
	require.NotNil(t, original)
	assert.Equal(t, 150, *original)
	task.PrivateData.BillingContext.ContractFact.RatioUnits = 80_000_000
	assert.Nil(t, TaskStatementOriginalQuota(task, delivery, LogTypeConsume))
}
