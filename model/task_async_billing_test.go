package model

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPrivateDataHistoricalJSONRemainsCompatible(t *testing.T) {
	var privateData TaskPrivateData
	err := privateData.Scan([]byte(`{"upstream_task_id":"old-task","billing_source":"wallet","billing_context":{"model_ratio":2,"origin_model_name":"old-model"}}`))

	require.NoError(t, err)
	assert.Equal(t, "old-task", privateData.UpstreamTaskID)
	require.NotNil(t, privateData.BillingContext)
	assert.Equal(t, 2.0, privateData.BillingContext.ModelRatio)
	assert.Nil(t, privateData.AsyncBilling)
}

func TestAttachAsyncTaskBillingStoresOnlyCompactProbe(t *testing.T) {
	privateData := TaskPrivateData{BillingContext: &TaskBillingContext{}}
	info := &relaycommon.RelayInfo{
		OriginModelName: "public-model",
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			ModelName: "public-model", EstimatedCompletionTokens: 100,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Authorization": "Bearer secret-token", "Cookie": "session=secret", "Content-Type": "application/json"},
			Body:    []byte(`{"_task":{"resolution":"1080p","has_video_input":true}}`),
		},
	}

	AttachAsyncTaskBilling(&privateData, info, 500)

	require.NotNil(t, privateData.AsyncBilling)
	assert.Equal(t, TaskBillingStatePending, privateData.AsyncBilling.State)
	assert.Equal(t, 100, privateData.AsyncBilling.EstimatedTokens)
	assert.Empty(t, privateData.AsyncBilling.BillingProbe.Headers, "must not persist request headers (Authorization/Cookie)")
	assert.NotContains(t, string(privateData.AsyncBilling.BillingProbe.Body), "prompt")
	assert.NotContains(t, string(privateData.AsyncBilling.BillingProbe.Body), "https://")
	assert.NotContains(t, string(privateData.AsyncBilling.BillingProbe.Body), "secret-token")
}

// TestAttachAsyncTaskBillingSkipsNonTieredTasks 验证普通（非 tiered_expr）任务不进入新计费状态机，
// 从而沿用原有 RecalculateTaskQuota/RefundTaskQuota/settleTaskBillingOnComplete 路径（方案 §5.2）。
func TestAttachAsyncTaskBillingSkipsNonTieredTasks(t *testing.T) {
	privateData := TaskPrivateData{BillingContext: &TaskBillingContext{OriginModelName: "doubao-seedance-2-0-260128"}}
	info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128"} // TieredBillingSnapshot == nil

	AttachAsyncTaskBilling(&privateData, info, 500)

	assert.Nil(t, privateData.AsyncBilling, "non-tiered tasks must keep the legacy billing path")
}

func TestAttachAsyncTaskBillingIncludesProtocolVideoTasks(t *testing.T) {
	privateData := TaskPrivateData{BillingContext: &TaskBillingContext{
		OriginModelName: "video-model",
		PerCallBilling:  true,
	}}
	info := &relaycommon.RelayInfo{
		OriginModelName: "video-model",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			ClientProtocol: TaskClientProtocolModelArkV3,
		},
	}

	AttachAsyncTaskBilling(&privateData, info, 500)

	require.NotNil(t, privateData.AsyncBilling)
	assert.Equal(t, TaskBillingStatePending, privateData.AsyncBilling.State)
	assert.True(t, privateData.BillingContext.PerCallBilling)
}

func TestTaskPrivateDataRoundTripsProviderBillingEvidence(t *testing.T) {
	privateData := TaskPrivateData{AsyncBilling: &TaskAsyncBillingContext{
		ActualTokens:        40594,
		ActualUsageReported: true,
		ActualUsageSource:   "usage.completion_tokens",
		ActualUsageEvidence: map[string]int{"usage.completion_tokens": 40594, "usage.prompt_tokens": 0},
		ProviderBillingEvidence: &relaycommon.ProviderBillingEvidence{
			Provider: "funcloud", TokenSource: "completionTokens", ReportedTokens: 40594,
			RawConsumption: "0.232731", ConsumptionUnit: "pointConsume",
			ProviderModel: "seedance-2-fast", Resolution: "720p",
		},
	}}
	value, err := privateData.Value()
	require.NoError(t, err)

	var restored TaskPrivateData
	require.NoError(t, restored.Scan(value))
	require.NotNil(t, restored.AsyncBilling)
	require.NotNil(t, restored.AsyncBilling.ProviderBillingEvidence)
	assert.True(t, restored.AsyncBilling.ActualUsageReported)
	assert.Equal(t, privateData.AsyncBilling.ActualUsageSource, restored.AsyncBilling.ActualUsageSource)
	assert.Equal(t, privateData.AsyncBilling.ActualUsageEvidence, restored.AsyncBilling.ActualUsageEvidence)
	assert.Equal(t, privateData.AsyncBilling.ProviderBillingEvidence, restored.AsyncBilling.ProviderBillingEvidence)
}
