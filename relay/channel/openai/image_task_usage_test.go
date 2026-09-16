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

// TestImageTaskUsageMapsOutputTokensDetailsToImageOutput locks the measured
// Azure Images usage shape (input 19 / output 196 with image_tokens=196).
// Before the output_tokens_details decode fix, img_o was always 0: expression
// pricing that charges image output separately (img_o) was billed 0 while the
// 196 tokens were carried entirely by the ordinary-output variable c.
func TestImageTaskUsageMapsOutputTokensDetailsToImageOutput(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":19,"output_tokens":196,"total_tokens":215,"input_tokens_details":{"text_tokens":19},"output_tokens_details":{"image_tokens":196}}}`)
	usage, err := ImageTaskUsage(nil, body)
	require.NoError(t, err)
	require.NotNil(t, usage)
	// output=196，img_o=196：明细进入统一输出明细，不再丢失。
	assert.Equal(t, 196, usage.CompletionTokens)
	assert.Equal(t, 196, usage.CompletionTokenDetails.ImageTokens)
	assert.Equal(t, 19, usage.PromptTokens)
	// 保留缺失与显式零：缺失的音频/文本输出明细保持零，不凭总数推断。
	assert.Zero(t, usage.CompletionTokenDetails.AudioTokens)
	assert.Zero(t, usage.CompletionTokenDetails.TextTokens)
	assert.Zero(t, usage.CompletionTokenDetails.ReasoningTokens)
}

// Both output-detail fields present: the image protocol's authoritative
// output_tokens_details wins by whole-object overwrite; representations are
// never added together.
func TestImageTaskUsageOutputDetailsPriorityNeverAddsDuplicates(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":10,"output_tokens":100,"completion_tokens_details":{"image_tokens":60,"text_tokens":40},"output_tokens_details":{"image_tokens":100}}}`)
	usage, err := ImageTaskUsage(nil, body)
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.CompletionTokenDetails.ImageTokens)
	assert.Zero(t, usage.CompletionTokenDetails.TextTokens)
	// 总量不重复相加：completion 保持 output_tokens 的覆盖语义。
	assert.Equal(t, 100, usage.CompletionTokens)
	assert.Equal(t, 110, usage.TotalTokens)
}
