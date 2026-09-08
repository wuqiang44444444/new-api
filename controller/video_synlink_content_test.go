package controller

import (
	"github.com/QuantumNous/new-api/setting/system_setting"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkLastFrameCredentialOnlyUsesFrozenOrigin(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProtocol:     dto.VideoUpstreamProtocolSynlinkVideoV1,
		VideoUpstreamQueryBaseURL: "https://frozen.example", Key: "frozen-key", UpstreamTaskID: "task/one",
		SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1),
		ClientRequest:            &model.TaskClientRequestSnapshot{ReturnLastFrame: common.GetPointer(true)},
	}}
	req, err := http.NewRequest(http.MethodGet, "https://untrusted.example", nil)
	require.NoError(t, err)
	source := "https://untrusted.example/steal-key"
	handled, err := applySynlinkLastFrameSource(task, req, &source)
	require.True(t, handled)
	require.NoError(t, err)
	assert.Equal(t, "https://frozen.example/v1/video/files/task%2Fone/last-frame", source)
	assert.Equal(t, "Bearer frozen-key", req.Header.Get("Authorization"))
	task.PrivateData.Key = ""
	_, err = applySynlinkLastFrameSource(task, req, &source)
	require.Error(t, err)
}

func TestSynlinkListAndDeleteKeepLocalTaskAndAppIsolation(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.AppID = 11
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.SubmitTime = common.GetTimestamp()
	task.CreatedAt = task.SubmitTime - 1
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolSynlinkVideoV1
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartySynlinkVideoV1
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1)
	require.NoError(t, model.DB.Save(task).Error)
	for _, appID := range []int{11, 12} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", appID)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks", nil)
		ModelArkVideoList(c)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Total int `json:"total"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		if appID == 11 {
			assert.Equal(t, 1, response.Total)
		} else {
			assert.Zero(t, response.Total)
		}
	}
	for _, tc := range []struct {
		status model.TaskStatus
		code   string
	}{
		{model.TaskStatusQueued, "cancellation_unsupported"},
		{model.TaskStatusInProgress, "task_running"},
		{model.TaskStatusSuccess, "delete_unsupported"},
	} {
		task.Status = tc.status
		require.NoError(t, model.DB.Save(task).Error)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", task.AppID)
		c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/v3/contents/generations/tasks/"+task.TaskID, nil)
		ModelArkVideoDelete(c)
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), tc.code)
		var persisted model.Task
		require.NoError(t, model.DB.First(&persisted, task.ID).Error)
		assert.Equal(t, tc.status, persisted.Status)
		assert.Zero(t, persisted.ClientDeletedAt)
		assert.Empty(t, persisted.CancellationState)
	}
}

func TestSynlinkLastFrameDeliveryUsesFrozenOriginAndRejectsRedirects(t *testing.T) {
	task := setupGenericTaskTest(t)
	allowPrivateTaskMediaTest(t)
	oldEvidence := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(oldEvidence) })
	var providerCalls, foreignCalls atomic.Int32
	var redirect atomic.Bool
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1); w.WriteHeader(500) }))
	t.Cleanup(foreign.Close)
	imageBytes := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		assert.Equal(t, "/v1/video/files/provider-tail/last-frame", r.URL.Path)
		assert.Equal(t, "Bearer frozen-key", r.Header.Get("Authorization"))
		if redirect.Load() {
			http.Redirect(w, r, foreign.URL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	t.Cleanup(provider.Close)
	task.AppID = 11
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.PrivateData = model.TaskPrivateData{
		VideoUpstreamProtocol:     dto.VideoUpstreamProtocolSynlinkVideoV1,
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartySynlinkVideoV1,
		VideoUpstreamQueryBaseURL: provider.URL, Key: "frozen-key", UpstreamTaskID: "provider-tail",
		SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1),
		ClientRequest:            &model.TaskClientRequestSnapshot{ReturnLastFrame: common.GetPointer(true)},
	}
	raw, err := common.Marshal(map[string]any{"content": map[string]string{"last_frame_url": foreign.URL}})
	require.NoError(t, err)
	task.Data = raw
	require.NoError(t, model.DB.Save(task).Error)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", task.ChannelId).Updates(map[string]any{"base_url": foreign.URL, "key": "rotated-key"}).Error)
	for _, scenario := range []string{"success", "redirect", "foreign_app", "not_requested"} {
		t.Run(scenario, func(t *testing.T) {
			redirect.Store(scenario == "redirect")
			if scenario == "not_requested" {
				task.PrivateData.ClientRequest.ReturnLastFrame = common.GetPointer(false)
				require.NoError(t, model.DB.Save(task).Error)
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", task.UserId)
			appID := task.AppID
			if scenario == "foreign_app" {
				appID++
			}
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID+"/content?part=last_frame", nil)
			require.True(t, proxyLinkVideoContent(c))
			switch scenario {
			case "success":
				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
				assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
				assert.Equal(t, imageBytes, recorder.Body.Bytes())
			case "foreign_app":
				assert.Equal(t, http.StatusNotFound, recorder.Code)
			default:
				assert.Equal(t, http.StatusBadGateway, recorder.Code)
			}
			assert.Empty(t, recorder.Header().Get("Location"))
			assert.NotContains(t, recorder.Body.String(), "frozen-key")
			assert.Zero(t, foreignCalls.Load())
		})
	}
	assert.Equal(t, int32(2), providerCalls.Load(), "only the two authorized content requests may reach the Provider")
}
