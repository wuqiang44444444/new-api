package openai

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageTaskUsagePreservesMissingAndExplicitZeroEvidence(t *testing.T) {
	absent, err := ImageTaskUsage(nil, []byte(`{}`))
	require.NoError(t, err)
	assert.Nil(t, absent)
	zero, err := ImageTaskUsage(nil, []byte(`{"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`))
	require.NoError(t, err)
	require.NotNil(t, zero)
	assert.Zero(t, zero.PromptTokens)
	assert.Zero(t, zero.TotalTokens)
}
