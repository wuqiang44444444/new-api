package model

import (
	"encoding/base64"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHistoricalTestReplayUsesFrozenPricesAndPreservesRecordedFee(t *testing.T) {
	facts := map[string]any{"model_ratio": 2, "completion_ratio": 3, "cache_ratio": 0.1, "model_price": -1, "group_ratio": 1, "cache_tokens": 40, "cache_write_tokens": 0, "request_path": "/v1/chat/completions", "request_conversion": []string{"OpenAI Compatible"}}
	raw, err := common.Marshal(facts)
	require.NoError(t, err)
	log := testLogForAmount(500)
	log.PromptTokens = 100
	log.CompletionTokens = 50
	log.Other = string(raw)
	amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
	require.True(t, amount.known)
	assert.True(t, amount.replayed)
	assert.Equal(t, "428", amount.original.String(), "(60 + 40*0.1 + 50*3)*2; old simplified fee was 500")
	assert.Equal(t, 500, log.Quota, "read-only correction must not rewrite the old fee")
	assert.EqualValues(t, 500, amount.recorded)
	delete(facts, "cache_write_tokens")
	raw, err = common.Marshal(facts)
	require.NoError(t, err)
	log.Other = string(raw)
	amount = upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
	assert.False(t, amount.known)
	assert.Equal(t, "missing_cache_write", amount.reason)
}

func TestHistoricalExpressionOriginalFromSuccessfulUndiscountedResult(t *testing.T) {
	expression := base64.StdEncoding.EncodeToString([]byte(`tier("base",p*2+cc*2.5+c*10)`))
	tests := []struct {
		name              string
		group             float64
		fee               int
		estimated, marker bool
		known             bool
	}{
		{"successful engine amount is already the original", 1, 220, false, true, true},
		{"zero result remains a known zero", 1, 0, false, true, true},
		{"nonunit group cannot be back-solved", 0.7, 220, false, true, false},
		{"estimate marker prevents confirmation", 1, 220, true, true, false},
		{"no successful engine marker", 1, 220, false, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts := map[string]any{"billing_mode": "tiered_expr", "expr_b64": expression, "group_ratio": tc.group, "cache_tokens": 0, "request_conversion": []string{"OpenAI Compatible"}, "admin_info": map[string]any{"local_count_tokens": tc.estimated}}
			if tc.marker {
				facts["matched_tier"] = "base"
			}
			raw, err := common.Marshal(facts)
			require.NoError(t, err)
			log := testLogForAmount(tc.fee)
			log.Other = string(raw)
			amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
			assert.Equal(t, tc.known, amount.known)
			if tc.known {
				assert.True(t, amount.recordedOriginal)
				assert.EqualValues(t, tc.fee, amount.original.IntPart())
			}
		})
	}
}

func TestHistoricalExpressionReplayOnlyRequiresUsedMeters(t *testing.T) {
	facts := map[string]any{"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(`tier("base",p*2+c*10)`)), "group_ratio": 0.5, "request_conversion": []string{"OpenAI Compatible"}, "matched_tier": "base"}
	raw, err := common.Marshal(facts)
	require.NoError(t, err)
	log := testLogForAmount(175)
	log.Other = string(raw)
	amount := upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
	require.True(t, amount.known)
	assert.True(t, amount.replayed)
	assert.Equal(t, "350", amount.original.String())
	log.Quota = 999
	amount = upstreamTestAmountFor(log, parseBillingReconciliationLog(log))
	assert.False(t, amount.known)
	assert.Equal(t, "recorded_amount_mismatch", amount.reason)
}
