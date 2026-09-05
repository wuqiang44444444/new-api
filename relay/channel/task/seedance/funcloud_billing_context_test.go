package seedance

import (
	"errors"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudBillingContextFailsClosedOnIncompleteFrozenFacts(t *testing.T) {
	tests := []struct {
		name    string
		context *relaycommon.VideoTaskBillingContext
	}{
		{name: "missing"},
		{name: "missing provider model", context: &relaycommon.VideoTaskBillingContext{
			BillingProbeBody: []byte(`{"_task":{"resolution":"720p","has_video_input":false}}`), EstimatedTokens: 100,
		}},
		{name: "missing probe", context: &relaycommon.VideoTaskBillingContext{ProviderModel: "seedance-2-fast", EstimatedTokens: 100}},
		{name: "missing frozen bound", context: &relaycommon.VideoTaskBillingContext{
			ProviderModel: "seedance-2-fast", BillingProbeBody: []byte(`{"_task":{"resolution":"720p","has_video_input":false}}`),
		}},
		{name: "missing video input fact", context: &relaycommon.VideoTaskBillingContext{
			ProviderModel: "seedance-2-fast", BillingProbeBody: []byte(`{"_task":{"resolution":"720p"}}`), EstimatedTokens: 100,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := funCloudTaskResponseContext(test.context)
			require.Error(t, err)
			var violation *relaycommon.UpstreamContractViolation
			assert.True(t, errors.As(err, &violation))
		})
	}
}

func TestFunCloudBillingContextUsesFrozenTypedFacts(t *testing.T) {
	context, err := funCloudTaskResponseContext(&relaycommon.VideoTaskBillingContext{
		ProviderModel:    " seedance-2-fast ",
		BillingProbeBody: []byte(`{"_task":{"resolution":"720P","has_video_input":true}}`),
		EstimatedTokens:  324000,
	})
	require.NoError(t, err)
	assert.Equal(t, "seedance-2-fast", context.ProviderModel)
	assert.Equal(t, "720p", context.Resolution)
	assert.True(t, context.HasVideoInput)
	// 信任上限不再复用预扣预算：720p 未冻结时长按 30s 上限推导（60k/s × 31s）。
	assert.Equal(t, 1_860_000, context.MaxTokens)
}

// 事故形状回归：1080p/15s 实测 731,025 tokens 曾超过预扣预算 520k 被永久拒绝；
// 合理性上限必须放行真实成功任务，同时仍拒绝巨量不可信数值。
func TestFunCloudTokenCeilingCoversMeasured1080pMaxDuration(t *testing.T) {
	context, err := funCloudTaskResponseContext(&relaycommon.VideoTaskBillingContext{
		ProviderModel:    "seedance-2",
		BillingProbeBody: []byte(`{"_task":{"resolution":"1080p","duration_seconds":15,"has_video_input":false}}`),
		EstimatedTokens:  520000,
	})
	require.NoError(t, err)
	assert.Equal(t, 1_920_000, context.MaxTokens)
	assert.Greater(t, context.MaxTokens, 731_025)

	// 短任务不因低速率被零值上限卡死。
	short, err := funCloudTaskResponseContext(&relaycommon.VideoTaskBillingContext{
		ProviderModel:    "seedance-2-fast",
		BillingProbeBody: []byte(`{"_task":{"resolution":"480p","duration_seconds":4,"has_video_input":false}}`),
		EstimatedTokens:  324000,
	})
	require.NoError(t, err)
	assert.Equal(t, 150_000, short.MaxTokens)
}
