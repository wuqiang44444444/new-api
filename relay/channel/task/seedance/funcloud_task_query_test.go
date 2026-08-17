package seedance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
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
	response, err := adaptor.FetchTask(server.URL, "funcloud-key", map[string]any{
		"task_id":                            "task-1",
		"video_upstream_profile":             kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
		"video_upstream_adapter_version":     "61:third_party_funcloud_seedance:v3",
		"video_upstream_query_path_template": "/api/v2/open/aigc/{task_id}",
		relaycommon.VideoTaskBillingContextKey: &relaycommon.VideoTaskBillingContext{
			ProviderModel:    "seedance-2",
			BillingProbeBody: []byte(`{"_task":{"resolution":"480p","has_video_input":false}}`),
			EstimatedTokens:  730000,
		},
	}, "")
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	result, err := adaptor.ParseTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, 40594, result.CompletionTokens)
	assert.Equal(t, 40594, result.TotalTokens)
	require.NotNil(t, result.ProviderBillingEvidence)
	assert.Equal(t, "completionTokens", result.ProviderBillingEvidence.TokenSource)
	assert.Equal(t, 40594, result.ProviderBillingEvidence.ReportedTokens)
}
