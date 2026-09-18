package service

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerBillingExpressionExplanationMatchesSettlement(t *testing.T) {
	oldQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
	const tokenExpr = `tier("base", p * 2 + c * 10)`
	const taskExpr = `tier("base", u("seconds") * 0.4)`
	// Reduced from the real backup: frozen expression and matched tier exist,
	// but historical task logs have no usable completion quantity.
	const legacyExpr = `param("_task.has_video_input") == true ? tier("480p720p_video", c * 6.16) : tier("480p720p", c * 10.26)`
	for _, tc := range []struct {
		name, expression, tier, extra string
		prompt, completion, quota     int
		qpu                           float64
		wantLines                     bool
		wantSubtotal                  string
	}{
		{"legacy task missing usage with known contract", legacyExpr, "480p720p", `"group_ratio":1,"contract_discount":1`, 0, 0, 4760640, 500000, false, ""},
		{"legacy task missing usage with unknown contract", legacyExpr, "480p720p", `"group_ratio":1`, 0, 0, 4760640, 500000, false, ""},
		{"zero cannot explain minimum charge", legacyExpr, "480p720p", `"group_ratio":1,"contract_applicable":false`, 0, 0, 1, 500000, false, ""},
		{"token group and contract", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5,"contract_discount":0.8`, 1000, 100, 600, 500000, true, "0.003"},
		{"token nondefault quota conversion", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5,"contract_discount":0.8`, 1000, 100, 1200, 1000000, true, "0.003"},
		{"token mismatched settlement", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5,"contract_discount":0.8`, 1000, 100, 900, 500000, false, ""},
		{"independent token components with incomplete contract", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5,"contract_id":7`, 1000, 100, 600, 500000, true, "0.003"},
		{"no recorded contract uses group factor", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5`, 1000, 100, 750, 500000, true, "0.003"},
		{"no recorded contract cannot explain mismatched settlement", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5`, 1000, 100, 600, 500000, false, ""},
		{"separate fee stays outside model components", tokenExpr, "base", `"usage_semantic":"openai","group_ratio":0.5,"contract_discount":0.8,"fee_quota":70`, 1000, 100, 670, 500000, true, "0.003"},
		{"task frozen seconds", taskExpr, "base", `"usage_units":{"seconds":"second"},"usage_facts":{"seconds":5},"group_ratio":0.5,"contract_discount":0.8`, 0, 0, 400000, 500000, true, "2"},
		{"task mismatched settlement", taskExpr, "base", `"usage_units":{"seconds":"second"},"usage_facts":{"seconds":5},"group_ratio":0.5,"contract_discount":0.8`, 0, 0, 700000, 500000, false, ""},
		{"task missing frozen seconds", taskExpr, "base", `"usage_units":{"seconds":"second"},"group_ratio":1,"contract_applicable":false`, 0, 0, 1000000, 500000, false, ""},
		{"explicit zero-priced output", `tier("base", c * 0)`, "base", `"group_ratio":1,"contract_applicable":false`, 0, 100, 0, 500000, true, "0"},
		{"constant charge with no tokens", `tier("base", c * 10 + 1000000)`, "base", `"group_ratio":1,"contract_applicable":false`, 0, 0, 500000, 500000, true, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.QuotaPerUnit = tc.qpu
			raw := `{"billing_mode":"tiered_expr","matched_tier":"` + tc.tier + `","expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(tc.expression)) + `",` + tc.extra + `}`
			log := &model.Log{Type: model.LogTypeConsume, Quota: tc.quota, PromptTokens: tc.prompt, CompletionTokens: tc.completion, Other: raw}
			// Exercise the actual response assembly, including the shared expression
			// projection, instead of validating a detached arithmetic helper.
			AttachLogsBillingDisplay([]*model.Log{log})
			var response struct {
				Explanation struct {
					Lines []customerBillingLine `json:"lines"`
				} `json:"billing_explanation"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &response))
			require.NotNil(t, response.Explanation.Lines, "the response contract requires an array, including unavailable explanations")
			assert.Equal(t, tc.quota, log.Quota)
			assert.Equal(t, tc.prompt, log.PromptTokens)
			assert.Equal(t, tc.completion, log.CompletionTokens)
			if !tc.wantLines {
				assert.Empty(t, response.Explanation.Lines)
				return
			}
			require.NotEmpty(t, response.Explanation.Lines)
			sum := decimal.Zero
			for _, line := range response.Explanation.Lines {
				sum = sum.Add(decimal.NewFromFloat(line.Subtotal))
			}
			want, err := decimal.NewFromString(tc.wantSubtotal)
			require.NoError(t, err)
			assert.True(t, sum.Equal(want), "component subtotal must match frozen metering")
			row := model.CustomerBillingLogRow(log)
			if row.HasAuxiliaryCharge {
				assert.Empty(t, row.OriginalEstimate, "model components must not reverse-discount the separate fee")
			} else if row.FinalRatio != nil {
				assert.True(t, sum.Mul(decimal.NewFromFloat(*row.FinalRatio)).Mul(decimal.NewFromFloat(tc.qpu)).Equal(decimal.NewFromInt(int64(tc.quota))))
			}
		})
	}
}
