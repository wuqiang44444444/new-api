package model

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

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
			name:       "frozen image tier in a mixed request-parameter expression",
			expression: `param("quality") == "high" ? tier("image", fixed(0.03)) : tier("token", p*3)`,
			fields:     `"matched_tier":"image","billing_unit":"request","fixed_price":0.03`,
			booked:     11250, original: 15000,
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

func TestFrozenBranchRetainsOriginalTokenNormalization(t *testing.T) {
	expr := `hour("UTC") > 6 ? tier("peak", p*2) : tier("off", p*2+cr)`
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 10, PromptTokens: 101, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.1,"contract_applicable":false,"usage_semantic":"openai","cache_tokens":1,"matched_tier":"peak","billing_unit":"token"}`, base64.StdEncoding.EncodeToString([]byte(expr)))}
	at := time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC)
	live, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), QuotaPerUnit: 500000, GroupRatio: 0.1}, billingexpr.TokenParams{P: 100, CR: 1, Len: 101}, billingexpr.RequestInput{PricingTime: &at})
	require.NoError(t, err)
	require.Equal(t, "peak", live.MatchedTier)
	require.Equal(t, log.Quota, live.ActualQuotaAfterGroup)
	original, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	require.Empty(t, reason)
	assert.EqualValues(t, live.ActualQuotaBeforeGroup, original)
}
func TestFrozenRequestRuleIsReplayable(t *testing.T) {
	expr := `tier("base",p*2)*(param("service_tier") == "fast" ? 2 : 1)`
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 100, PromptTokens: 100, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.5,"contract_applicable":false,"usage_semantic":"openai","matched_tier":"base","billing_unit":"token","request_rules":[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":2,"matched":true}]}`, base64.StdEncoding.EncodeToString([]byte(expr)))}
	original, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	require.Empty(t, reason)
	assert.EqualValues(t, 200, original)
	for _, altered := range []string{
		strings.Replace(log.Other, `"multiplier":2`, `"multiplier":3`, 1),
		strings.Replace(log.Other, `"matched":true`, `"matched":false`, 1),
	} {
		changed := log
		changed.Other = altered
		_, reason := replayCustomerOriginal(changed, parseBillingReconciliationLog(changed))
		assert.NotEmpty(t, reason)
	}
}

func TestFrozenReplayUsesCacheWriterEvidence(t *testing.T) {
	expr := base64.StdEncoding.EncodeToString([]byte(`tier("cache", p*2+cc*4)`))
	for _, tc := range []struct {
		name, fields     string
		booked, original int
		reason           string
	}{
		{"two aliases use the larger count", `,"cache_write_tokens":2,"cached_creation_tokens":5`, 53, 105, ""},
		{"historical writer alias uses the same rule", `,"cache_write_tokens":2,"cache_creation_tokens":5`, 53, 105, ""},
		{"explicit zero", `,"cache_write_tokens":0`, 50, 100, ""},
		{"unproven omission", `,"cache_write_tokens_reported":false`, 50, 0, BillingOriginalReplayMissingUsage},
		{"native zero omission", `,"request_path":"/v1/chat/completions","model_ratio":1,"model_price":-1,"completion_ratio":1,"user_group_ratio":0,"cache_tokens":0,"cache_ratio":1`, 50, 100, ""},
		{"frozen values override aliases", `,"cache_write_tokens":100,"billing_tokens":{"p":98,"cc":2}`, 51, 102, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, PromptTokens: 100, Quota: tc.booked, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"billing_unit":"token","matched_tier":"cache","usage_semantic":"openai","group_ratio":0.5,"contract_applicable":false%s}`, expr, tc.fields)}
			original, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.reason, reason)
			assert.EqualValues(t, tc.original, original)
		})
	}
}

func TestFrozenFixedPriceRequestRulePreservesImageCount(t *testing.T) {
	expression := `image_count * tier("image", fixed(0.04)) * (param("quality") == "high" ? 2 : 1)`
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 40000, Other: fmt.Sprintf(
		`{"billing_mode":"tiered_expr","billing_unit":"request","expr_b64":%q,"matched_tier":"image","group_ratio":0.5,"contract_applicable":false,"image_count":2,"fixed_price":0.04,"request_rules":[{"cond":"param(\"quality\") == \"high\"","multiplier":2,"matched":true}]}`,
		base64.StdEncoding.EncodeToString([]byte(expression)))}
	original, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	require.Empty(t, reason)
	assert.EqualValues(t, 80000, original)
	log.Other = strings.Replace(log.Other, `"fixed_price":0.04`, `"fixed_price":0.05`, 1)
	_, reason = replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Equal(t, BillingOriginalReplayMismatch, reason)
}

func TestFrozenRequestRuleRequiresExplicitOutcome(t *testing.T) {
	expression := `tier("base",p*2)*(param("service_tier") == "fast" ? 1.01 : 1)`
	for _, tc := range []struct {
		name, field, reason string
		amount              int
	}{
		{"true", `,"matched":true`, "", 101},
		{"false", `,"matched":false`, "", 100},
		{"absent", "", BillingOriginalReplayMissingContext, 0},
		{"null", `,"matched":null`, BillingOriginalReplayMissingContext, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 10, PromptTokens: 100, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.1,"contract_applicable":false,"usage_semantic":"openai","matched_tier":"base","billing_unit":"token","request_rules":[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":1.01%s}]}`, base64.StdEncoding.EncodeToString([]byte(expression)), tc.field)}
			amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.reason, reason)
			assert.EqualValues(t, tc.amount, amount)
		})
	}
}

func TestFrozenReplayComposesFixedTierAndRequestRule(t *testing.T) {
	expression := `(param("quality") == "high" ? tier("high", fixed(0.04)) : tier("low", fixed(0.02))) * image_count * (param("service_tier") == "fast" ? 2 : 1)`
	imageCount := 2
	params := billingexpr.TokenParams{}
	req := billingexpr.RequestInput{Body: []byte(`{"quality":"high","service_tier":"fast"}`), ImageCount: &imageCount}
	live, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 500000, GroupRatio: 0.5}, params, req)
	require.NoError(t, err)
	require.Equal(t, "high", live.MatchedTier)
	require.Equal(t, 40000, live.ActualQuotaAfterGroup)
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: live.ActualQuotaAfterGroup, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","billing_unit":"request","expr_b64":%q,"matched_tier":"high","group_ratio":0.5,"contract_applicable":false,"image_count":2,"fixed_price":0.04,"request_rules":[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":2,"matched":true}]}`, base64.StdEncoding.EncodeToString([]byte(expression)))}
	amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Empty(t, reason)
	assert.EqualValues(t, live.ActualQuotaBeforeGroup, amount)
	for _, tc := range []struct {
		name, before, after, reason string
	}{
		{"missing result", `,"matched":true`, "", BillingOriginalReplayMissingContext},
		{"wrong price", `"fixed_price":0.04`, `"fixed_price":0.05`, BillingOriginalReplayMissingContext},
		{"missing count", `,"image_count":2`, "", BillingOriginalReplayMissingUsage},
		{"wrong result", `"matched":true`, `"matched":false`, BillingOriginalReplayMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := log
			changed.Other = strings.Replace(log.Other, tc.before, tc.after, 1)
			_, reason := replayCustomerOriginal(changed, parseBillingReconciliationLog(changed))
			assert.Equal(t, tc.reason, reason)
		})
	}
}

func TestFrozenReplayKeepsRuleIndicesAndRejectsAmbiguousFixedTiers(t *testing.T) {
	const high = `tier("high", fixed(0.04))`
	const base = `(param("quality") == "high" ? tier("high", fixed(0.04)) : tier("low", fixed(0.02)) * (param("size") == "large" ? 3 : 1)) * (param("service_tier") == "fast" ? 2 : 1)`
	for _, tc := range []struct {
		name, expression, reason string
	}{
		{"discarded branch rule", base, ""},
		{"same tier and price in both branches", strings.Replace(base, `tier("low", fixed(0.02))`, high, 1), BillingOriginalReplayMissingContext},
		{"literal false selector", strings.Replace(base, `param("quality") == "high"`, "false", 1), BillingOriginalReplayMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 20000, Other: fmt.Sprintf(
				`{"billing_mode":"tiered_expr","billing_unit":"request","expr_b64":%q,"matched_tier":"high","group_ratio":0.5,"contract_applicable":false,"fixed_price":0.04,"request_rules":[{"cond":"param(\"size\") == \"large\"","multiplier":3,"matched":false},{"cond":"param(\"service_tier\") == \"fast\"","multiplier":2,"matched":true}]}`,
				base64.StdEncoding.EncodeToString([]byte(tc.expression)))}
			amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.reason, reason)
			if tc.reason == "" {
				assert.EqualValues(t, 40000, amount)
				log.Other = strings.Replace(log.Other, `"matched":false`, `"matched":true`, 1)
				_, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
				assert.Equal(t, BillingOriginalReplayMismatch, reason)
			}
		})
	}
}

func TestFrozenRuleRejectsDuplicateEvidenceFields(t *testing.T) {
	expr := `tier("base",p*2)*(param("service_tier") == "fast" ? 1.01 : 1)`
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 10, PromptTokens: 100, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.1,"contract_applicable":false,"usage_semantic":"openai","matched_tier":"base","billing_unit":"token","request_rules":[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":1.01,"matched":true,"matched":false}]}`, base64.StdEncoding.EncodeToString([]byte(expr)))}
	for _, tc := range []struct{ name, replacement string }{
		{"conflicting outcome", `"matched":true,"matched":false`},
		{"escaped duplicate outcome", `"matched":true,"match\u0065d":false`},
		{"duplicate multiplier", `"multiplier":2,"matched":false`},
		{"duplicate condition", `"cond":"unrelated","matched":false`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := log
			row.Other = strings.Replace(log.Other, `"matched":true,"matched":false`, tc.replacement, 1)
			original, reason := replayCustomerOriginal(row, parseBillingReconciliationLog(row))
			assert.Equal(t, BillingOriginalReplayMissingContext, reason)
			assert.Zero(t, original)
		})
	}
}

func TestFrozenFixedTierCannotOverrideFrozenMeterCondition(t *testing.T) {
	expr := `(p > 5 && param("quality") == "high" ? tier("high",fixed(0.04)) : tier("low",fixed(0.02))) * (param("service_tier") == "fast" ? 2 : 1)`
	live, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), QuotaPerUnit: 500000, GroupRatio: 0.00003}, billingexpr.TokenParams{P: 0}, billingexpr.RequestInput{Body: []byte(`{"quality":"high","service_tier":"fast"}`)})
	require.NoError(t, err)
	require.Equal(t, "low", live.MatchedTier)
	require.Equal(t, 1, live.ActualQuotaAfterGroup)
	require.EqualValues(t, 20000, live.ActualQuotaBeforeGroup)
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 1, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.00003,"contract_applicable":false,"matched_tier":"high","fixed_price":0.04,"billing_unit":"request","billing_tokens":{"p":0},"request_rules":[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":2,"matched":true}]}`, base64.StdEncoding.EncodeToString([]byte(expr)))}
	original, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Equal(t, BillingOriginalReplayMismatch, reason)
	assert.Zero(t, original)

	log.Other = strings.Replace(log.Other, `"p":0`, `"p":6`, 1)
	original, reason = replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Empty(t, reason)
	assert.EqualValues(t, 40000, original)

	// Expressions with no traced multipliers use the same branch constraint.
	log.Other = strings.Replace(log.Other, `"p":6`, `"p":0`, 1)
	log.Other = strings.Replace(log.Other, base64.StdEncoding.EncodeToString([]byte(expr)),
		base64.StdEncoding.EncodeToString([]byte(strings.Split(expr, " * ")[0])), 1)
	log.Other = strings.Split(log.Other, `,"request_rules"`)[0] + "}"
	original, reason = replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Equal(t, BillingOriginalReplayMismatch, reason)
	assert.Zero(t, original)
}

func TestFrozenReplayRejectsDuplicateRequestRulesContainers(t *testing.T) {
	expr := `tier("base",p*2)*(param("service_tier") == "fast" ? 1.01 : 1)`
	trace := `[{"cond":"param(\"service_tier\") == \"fast\"","multiplier":1.01,"matched":true}]`
	rules := `"request_rules":` + trace
	duplicate := rules + `,"request_rules":` + strings.Replace(trace, "true", "false", 1)
	for _, tc := range []struct {
		name, fields string
		duplicate    bool
	}{
		{"top level duplicate", duplicate, true},
		{"escaped duplicate", rules + `,"request_\u0072ules":null`, true},
		{"frozen snapshot duplicate", `"statement_snapshot":{` + duplicate + "}", true},
		{"admin snapshot duplicate", `"admin_info":{"statement_snapshot":{` + duplicate + "}}", true},
		{"single top level", rules, false},
		{"single snapshot", `"statement_snapshot":{` + rules + "}", false},
		{"single admin snapshot", `"admin_info":{"statement_snapshot":{` + rules + "}}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 10, PromptTokens: 100, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.1,"contract_applicable":false,"usage_semantic":"openai","matched_tier":"base","billing_unit":"token",%s}`, base64.StdEncoding.EncodeToString([]byte(expr)), tc.fields)}
			amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
			if tc.duplicate {
				assert.Equal(t, BillingOriginalReplayMissingContext, reason)
				assert.Zero(t, amount)
			} else {
				assert.Empty(t, reason)
				assert.EqualValues(t, 101, amount)
			}
		})
	}
}

func TestFrozenReplayRejectsTierAndRuleConflict(t *testing.T) {
	expr := `(param("quality") == "high" ? tier("high",fixed(0.04)) : tier("low",fixed(0.02))) * (param("quality") == "high" ? 1.01 : 1)`
	snapshot := billingexpr.BillingSnapshot{ExprString: expr, ExprHash: billingexpr.ExprHashString(expr), QuotaPerUnit: 500000, GroupRatio: 0.00002}
	live, err := billingexpr.ComputeTieredQuotaWithRequest(&snapshot, billingexpr.TokenParams{}, billingexpr.RequestInput{Body: []byte(`{"quality":"low"}`)})
	require.NoError(t, err)
	require.Equal(t, "low", live.MatchedTier)
	require.EqualValues(t, 10000, live.ActualQuotaBeforeGroup)
	require.Zero(t, live.ActualQuotaAfterGroup)
	log := billingReconciliationLog{UserId: 1, Type: LogTypeConsume, Quota: 0, Other: fmt.Sprintf(`{"billing_mode":"tiered_expr","expr_b64":%q,"group_ratio":0.00002,"contract_applicable":false,"matched_tier":"high","fixed_price":0.04,"billing_unit":"request","request_rules":[{"cond":"param(\"quality\") == \"high\"","multiplier":1.01,"matched":false}]}`, base64.StdEncoding.EncodeToString([]byte(expr)))}
	amount, reason := replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Equal(t, BillingOriginalReplayMismatch, reason)
	assert.Zero(t, amount)

	log.Other = strings.Replace(log.Other, `"matched":false`, `"matched":true`, 1)
	amount, reason = replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Empty(t, reason)
	assert.EqualValues(t, 20200, amount)

	// The same predicate is traced in a branch that did not execute.
	skipped := `param("quality") == "high" ? tier("high",fixed(0.04)) : tier("low",fixed(0.02)) * (param("quality") == "high" ? 1.01 : 1)`
	log.Other = strings.Replace(log.Other, base64.StdEncoding.EncodeToString([]byte(expr)), base64.StdEncoding.EncodeToString([]byte(skipped)), 1)
	log.Other = strings.Replace(log.Other, `"matched":true`, `"matched":false`, 1)
	amount, reason = replayCustomerOriginal(log, parseBillingReconciliationLog(log))
	assert.Empty(t, reason)
	assert.EqualValues(t, 20000, amount)
}
