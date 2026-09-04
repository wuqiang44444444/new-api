package seedance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudFetchTaskUsesAIGCQueryAndReportedCompletionTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/api/v2/open/aigc/task-1", request.URL.Path)
		assert.Equal(t, "Bearer funcloud-key", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{"taskId":"task-1","status":"success","result":["https://cdn.example.com/video.mp4"],"completionTokens":40594,"pointConsume":"0.232731"}}`))
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{ChannelType: constant.ChannelTypeSeedanceLink}
	task := &model.Task{
		TaskID: "task_public_1",
		Properties: model.Properties{
			UpstreamModelName: "seedance-2",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID:                 "task-1",
			VideoUpstreamProfile:           kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
			SouthboundAdapterVersion:       relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedance),
			VideoUpstreamQueryPathTemplate: "/api/v2/open/aigc/{task_id}",
			AsyncBilling: &model.TaskAsyncBillingContext{
				EstimatedTokens: 730000,
				BillingProbe: &billingexpr.RequestInput{
					Body: []byte(`{"_task":{"resolution":"480p","has_video_input":false}}`),
				},
			},
		},
	}
	response, err := adaptor.FetchTask(server.URL, "funcloud-key", task, "")
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	result, err := adaptor.ParseTaskResult(task, response, body)
	require.NoError(t, err)
	assert.Equal(t, 40594, result.CompletionTokens)
	assert.Equal(t, 40594, result.TotalTokens)
	require.NotNil(t, result.ProviderBillingEvidence)
	assert.Equal(t, "completionTokens", result.ProviderBillingEvidence.TokenSource)
	assert.Equal(t, 40594, result.ProviderBillingEvidence.ReportedTokens)
}
