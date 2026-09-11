package seedance

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkMappedWireAndPublishedContract(t *testing.T) {
	for _, providerModel := range kitdto.SynlinkVideoModels() {
		req := providerTestRequest()
		req.Model = "customer-video"
		req.GenerateAudio = common.GetPointer(false)
		req.Watermark = common.GetPointer(false)
		req.ReturnLastFrame = common.GetPointer(true)
		req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "https://cdn.example/image.png?x=1&y=2"}})
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		pinSeedanceExtensionForTest(t, c)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
		relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, IsModelMapped: true, UpstreamModelName: providerModel, ChannelBaseUrl: "https://provider.example", ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1}}}
		a := &TaskAdaptor{}
		a.Init(info)
		require.Nil(t, a.ValidateMappedRequest(c, info))
		reader, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		raw, err := io.ReadAll(reader)
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, common.Unmarshal(raw, &wire))
		assert.Equal(t, providerModel, wire["model"])
		assert.Equal(t, false, wire["generate_audio"])
		assert.Equal(t, false, wire["watermark"])
		assert.Equal(t, true, wire["return_last_frame"])
		content := wire["content"].([]any)
		image := content[len(content)-1].(map[string]any)["image_url"].(map[string]any)
		assert.Equal(t, "https://cdn.example/image.png?x=1&y=2", image["url"])
		assert.NotContains(t, wire, "real_person_mode")
		assert.NotContains(t, wire, "omni_reference_task_type")
		assert.Equal(t, "customer-video", req.Model)
		address, err := a.BuildRequestURL(info)
		require.NoError(t, err)
		assert.Equal(t, "https://provider.example/v1/video/generate", address)
		api, ok := publishedVideoFixture(req.Model, dto.VideoUpstreamProtocolSynlinkVideoV1, providerModel, false)
		require.True(t, ok)
		for _, op := range api.Operations {
			if op.Operation == "list_videos" {
				assert.True(t, op.Supported)
			}
			if op.Operation == "delete_video" {
				assert.False(t, op.Supported)
			}
		}
	}
}

func TestSynlinkRejectsUnsupportedReferencesAndUnsafeDuration(t *testing.T) {
	for _, ref := range []string{"asset://provider-id", "data:image/png;base64,aA=="} {
		req := providerTestRequest()
		req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: ref}, Role: common.GetPointer("reference_image")})
		assert.Error(t, validateSynlinkRequest(kitdto.SynlinkVideoModels()[0], req))
	}
	for _, duration := range []int{-1, 0, relaycommon.MaxTaskDurationSeconds + 1} {
		req := providerTestRequest()
		req.Duration = &duration
		assert.Error(t, validateSynlinkRequest(kitdto.SynlinkVideoModels()[0], req))
	}
	req := providerTestRequest()
	req.CallbackURL = common.GetPointer("https://callback.example")
	assert.Error(t, validateSynlinkRequest(kitdto.SynlinkVideoModels()[0], req))
	_, err := buildSynlinkRequest(nil, &requestPayload{Content: []ContentItem{{Type: "image_url", ImageURL: &MediaURL{URL: "asset://fhas_unresolved"}}}})
	assert.Error(t, err)
}

func TestSynlinkPollingAndLastFrameUseFrozenFacts(t *testing.T) {
	previous := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(previous) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/video/tasks/syn-1", r.URL.Path)
		assert.Equal(t, "Bearer frozen-key", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"task":{"id":"syn-1","status":"completed","outputs":["https://cdn.example/video.mp4"],"usage":{"completion_tokens":0,"total_tokens":0}}}`)
	}))
	defer server.Close()
	profile := dto.VideoUpstreamProfileThirdPartySynlinkVideoV1
	task := &model.Task{PrivateData: model.TaskPrivateData{UpstreamTaskID: "syn-1", Key: "frozen-key", VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1, VideoUpstreamProfile: profile, VideoUpstreamQueryPathTemplate: "/v1/video/tasks/{task_id}", SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile), ClientRequest: &model.TaskClientRequestSnapshot{ReturnLastFrame: common.GetPointer(true)}}}
	a := &TaskAdaptor{ChannelType: constant.ChannelTypeSeedanceLink}
	resp, err := a.FetchTaskWithContext(context.Background(), server.URL, "changed-key", task, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"last_frame_url":"/v1/video/files/syn-1/last-frame"`)
	info, err := a.ParseTaskResult(task, resp, raw)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, info.Status)
	assert.True(t, info.UsageReported)
	assert.Zero(t, info.CompletionTokens)
	caps := a.TaskLifecycleCapabilities(task)
	assert.False(t, caps.SupportsCancelQueued)
	assert.False(t, caps.SupportsDeleteTerminal)
	task.PrivateData.SouthboundAdapterVersion = ""
	_, err = a.FetchTaskWithContext(context.Background(), server.URL, "changed-key", task, "")
	assert.Error(t, err)
}
