package service

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPreviewUsesSettlementUnitsAndFullExpression(t *testing.T) {
	for _, expression := range []string{
		`(u("video") ? tier("video", u("tokens") * 5) : tier("plain", u("tokens") * 8)) / 1000000`,
		`tier("lookup", u("tokens") * {"720p":5,"1080p":8}[u("resolution")] / 1000000)`,
		`tier("constant", 2.5)`,
	} {
		t.Run(expression, func(t *testing.T) {
			facts := map[string]any{"video": true, "tokens": float64(1000000), "resolution": "720p"}
			got := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: expression, TaskUsage: true, Sample: &BillingExprPreviewSample{Usage: facts}}})[0]
			require.Empty(t, got.Error)
			require.NotNil(t, got.Evaluation)
			want, err := billingexpr.ComputeTieredQuotaWithRequest(&billingexpr.BillingSnapshot{ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), TaskUsageBilling: true, QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1}, billingexpr.TokenParams{}, billingexpr.RequestInput{Usage: facts})
			require.NoError(t, err)
			assert.Equal(t, want.ActualQuotaAfterGroup, got.Evaluation.Quota)
			assert.Equal(t, want.ActualQuotaBeforeGroup/common.QuotaPerUnit, got.Evaluation.RawCostUSD)
			assert.Equal(t, want.MatchedTier, got.Evaluation.MatchedTier)
		})
	}
}

func TestTaskPreviewFailsClosedAndDoesNotReinterpretLegacy(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		facts            map[string]any
	}{
		{"legacy", `tier("base", c*5)`, map[string]any{}},
		{"missing boolean", `u("video") == true ? 5 : 8`, map[string]any{}},
		{"negative", `-1`, map[string]any{}},
		{"nonfinite result", `1.0/0.0`, map[string]any{}},
		{"nonfinite sample", `u("tokens")`, map[string]any{"tokens": math.NaN()}},
		{"large sample", `u("tokens")`, map[string]any{"tokens": float64(math.MaxInt32) + 1}},
		{"nested sample", `u("tokens")`, map[string]any{"tokens": map[string]any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: tc.expression, TaskUsage: true, Sample: &BillingExprPreviewSample{Usage: tc.facts}}})[0]
			assert.NotEmpty(t, got.Error)
			assert.Nil(t, got.Evaluation)
		})
	}
	legacy := PreviewBillingExpressions([]BillingExprPreviewItem{{Expression: `tier("base", c*5)`, Sample: &BillingExprPreviewSample{CompletionTokens: 1000000}}})[0]
	require.Empty(t, legacy.Error)
	assert.Equal(t, float64(5), legacy.Evaluation.RawCostUSD)
}
