package thirdparty

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInvalidUsageCannotFallThroughToTotalCharge(t *testing.T) {
	for _, input := range []string{
		`{"usage":{"completion_tokens":-1,"total_tokens":100}}`,
		`{"usage":{"completion_tokens":2147483648,"total_tokens":100}}`,
		`{"usage":{"completion_tokens":1.5,"total_tokens":100}}`,
		`{"usage":{"prompt_tokens":-1,"total_tokens":100}}`,
		`{"usage":{"prompt_tokens":"invalid","total_tokens":100}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(input), &payload))
			usage := normalizeTerminalTokenUsage(payload)
			assert.Nil(t, usage.Usage)
			assert.Equal(t, 100, usage.Evidence["usage.total_tokens"])
		})
	}
}

func TestUsageDetailObjectsDoNotInvalidateStandaloneTotal(t *testing.T) {
	var payload map[string]any
	require.NoError(t, common.Unmarshal([]byte(`{"usage":{"completion_tokens_details":{"reasoning_tokens":0},"prompt_tokens_details":{"cached_tokens":0},"total_tokens":100}}`), &payload))
	usage := normalizeTerminalTokenUsage(payload)
	require.NotNil(t, usage.Usage)
	assert.Equal(t, 100, usage.Usage["completion_tokens"])
	assert.Equal(t, "usage.total_tokens", usage.Source)
}
