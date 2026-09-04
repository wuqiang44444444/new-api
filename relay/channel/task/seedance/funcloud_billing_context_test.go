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
	assert.Equal(t, 324000, context.MaxTokens)
}
