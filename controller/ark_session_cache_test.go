package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkSessionContextCountsParentInputOutputAndNewInput(t *testing.T) {
	assert.True(t, arkSessionContextWithinLimit(7000+512, 424))
	assert.False(t, arkSessionContextWithinLimit(7000+512, 425))
	assert.False(t, arkSessionContextWithinLimit(8192+16, 5))
	assert.True(t, arkSessionContextWithinLimit(0, 7936))
}

func TestArkCacheReceiptRetriesTransientLocalFailureWithSameReceipt(t *testing.T) {
	row := model.ArkPrefixCache{ID: "local", ProviderID: "resp_upstream", State: model.ArkCacheInferencePending}
	attempts := 0
	err := persistArkCacheReceipt(t.Context(), row, func(ctx context.Context, got model.ArkPrefixCache) error {
		assert.Equal(t, row, got)
		_, deadline := ctx.Deadline()
		assert.True(t, deadline)
		attempts++
		if attempts == 1 {
			return errors.New("database unavailable")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestArkSessionInferenceRequiresReportedCacheUsage(t *testing.T) {
	for _, tt := range []struct {
		name  string
		meter string
		ok    bool
	}{
		{"explicit zero", `{"input_tokens":8,"output_tokens":3,"input_tokens_details":{"cached_tokens":0}}`, true},
		{"cache hit", `{"input_tokens":8,"output_tokens":3,"input_tokens_details":{"cached_tokens":6}}`, true},
		{"missing hit field", `{"input_tokens":8,"output_tokens":3,"input_tokens_details":{}}`, false},
		{"missing output", `{"input_tokens":8,"input_tokens_details":{"cached_tokens":6}}`, false},
		{"hit exceeds input", `{"input_tokens":8,"output_tokens":3,"input_tokens_details":{"cached_tokens":9}}`, false},
		{"output exceeds request", `{"input_tokens":8,"output_tokens":17,"input_tokens_details":{"cached_tokens":6}}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usage := arkSessionInferenceUsage(common.RawMessage(tt.meter), 16)
			if !tt.ok {
				require.Nil(t, usage)
				return
			}
			require.NotNil(t, usage)
			assert.Equal(t, 8, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 11, usage.TotalTokens)
		})
	}
}
