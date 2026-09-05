package funcloud

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskResponseUsesProviderReportedCompletionTokens(t *testing.T) {
	body := []byte(`{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"0.232731","output":{"pointConsume":"0.2327310"}}}`)
	normalized, err := TaskResponse(body, "task-1", TaskResponseContext{
		ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000,
	})
	require.NoError(t, err)

	var response struct {
		Status                  string                               `json:"status"`
		ProviderBillingEvidence *relaycommon.ProviderBillingEvidence `json:"_provider_billing_evidence"`
		Usage                   struct {
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(normalized, &response))
	assert.Equal(t, "succeeded", response.Status)
	assert.Equal(t, 40594, response.Usage.CompletionTokens)
	assert.Equal(t, 40594, response.Usage.TotalTokens)
	require.NotNil(t, response.ProviderBillingEvidence)
	assert.Equal(t, "completionTokens", response.ProviderBillingEvidence.TokenSource)
	assert.Equal(t, 40594, response.ProviderBillingEvidence.ReportedTokens)
	assert.Equal(t, "0.232731", response.ProviderBillingEvidence.RawConsumption)
}

func TestTaskResponseAllowsMissingPointConsumeWhenCompletionTokensAreTrusted(t *testing.T) {
	body := []byte(`{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594}}`)
	normalized, err := TaskResponse(body, "task-1", TaskResponseContext{
		ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000,
	})
	require.NoError(t, err)

	var response struct {
		ProviderBillingEvidence *relaycommon.ProviderBillingEvidence `json:"_provider_billing_evidence"`
		Usage                   struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	require.NoError(t, common.Unmarshal(normalized, &response))
	assert.Equal(t, 40594, response.Usage.CompletionTokens)
	require.NotNil(t, response.ProviderBillingEvidence)
	assert.Equal(t, "completionTokens", response.ProviderBillingEvidence.TokenSource)
	assert.Equal(t, 40594, response.ProviderBillingEvidence.ReportedTokens)
	assert.Empty(t, response.ProviderBillingEvidence.RawConsumption)
	assert.Empty(t, response.ProviderBillingEvidence.ConsumptionUnit)
}

func TestTaskResponseFailsClosedWhenCompletionTokensAreUntrustworthy(t *testing.T) {
	baseContext := TaskResponseContext{ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000}
	tests := []struct {
		name string
		body string
	}{
		{name: "missing with pointConsume present", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"pointConsume":"0.232731"}}`},
		{name: "string", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":"40594"}}`},
		{name: "fractional", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594.5}}`},
		{name: "zero", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":0}}`},
		{name: "negative", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":-1}}`},
		{name: "exceeds frozen bound", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":100001}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := TaskResponse([]byte(test.body), "task-1", baseContext)
			require.Error(t, err)
			var violation *relaycommon.UpstreamContractViolation
			assert.True(t, errors.As(err, &violation))
		})
	}
}

func TestTaskResponseFailsClosedWhenReportedPointConsumeIsUntrustworthy(t *testing.T) {
	baseContext := TaskResponseContext{ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000}
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"not-a-number"}}`},
		{name: "NaN", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"NaN"}}`},
		{name: "negative", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"-1"}}`},
		{name: "zero", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"0"}}`},
		{name: "positive infinity", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"+Inf"}}`},
		{name: "conflict", body: `{"code":0,"data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"0.1","output":{"pointConsume":"0.2"}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := TaskResponse([]byte(test.body), "task-1", baseContext)
			require.Error(t, err)
			var violation *relaycommon.UpstreamContractViolation
			assert.True(t, errors.As(err, &violation))
		})
	}
}

func TestTaskResponseDoesNotRequireUsageForFailedTasks(t *testing.T) {
	failed := []byte(`{"code":0,"data":{"taskId":"task-1","status":"failed","errorCode":"UPSTREAM_FAILED","errorMsg":"failed"}}`)
	normalized, err := TaskResponse(failed, "task-1", TaskResponseContext{
		ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000,
	})
	require.NoError(t, err)
	assert.NotContains(t, string(normalized), "completion_tokens")
}

// Funcloud 以 HTTP 200 + 业务码 30003 表示任务不存在（2026-09-04 实测验证）。
// 这是可采信的终态观测：必须映射为确定性 not-found（有界宽限后 FAILURE + 退款），
// 而不是当作合同违规无限 reconciliation。
func TestTaskResponseMapsBusinessNotFoundCodeToUpstreamTaskNotFound(t *testing.T) {
	_, err := TaskResponse([]byte(`{"code":30003,"msg":"任务不存在","data":null}`), "task-1", TaskResponseContext{
		ProviderModel: "seedance-2-fast", Resolution: "720p", MaxTokens: 100000,
	})
	require.Error(t, err)
	var notFound *relaycommon.UpstreamTaskNotFound
	assert.True(t, errors.As(err, &notFound))
	assert.Equal(t, 30003, notFound.ProviderCode)
}
