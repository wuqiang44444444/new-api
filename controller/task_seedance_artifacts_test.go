package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSeedanceArtifactTest(t *testing.T, baseURL string) *model.Task {
	t.Helper()
	task := setupGenericTaskTest(t)
	previousSecret := common.CryptoSecret
	previousEvidence := system_setting.GetTaskRequestEvidenceConfig()
	common.CryptoSecret = "seedance-artifact-test-secret"
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		system_setting.SetTaskRequestEvidenceConfig(previousEvidence)
	})
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.AppID = 11
	task.Action = constant.TaskActionReferenceToVideo
	task.PrivateData = model.TaskPrivateData{
		ResultURL: baseURL + "/video.mp4", Key: "frozen-key", UpstreamTaskID: "provider-task",
		VideoUpstreamQueryBaseURL: baseURL,
		VideoUpstreamProtocol:     dto.VideoUpstreamProtocolMoxingModelArkV1,
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyMoxingModelArk,
		SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(
			constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyMoxingModelArk),
		Execution: &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{
			Key: "seedance-link", Name: "Seedance Link", Version: "1.3.0",
		}},
	}
	require.NoError(t, model.DB.Save(task).Error)
	return task
}

func TestSeedanceDashboardArtifactsDeliverFrozenVideo(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "/video.mp4", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"), "media URLs must not receive channel credentials")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "10")
			return
		}
		assert.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("vide"))
	}))
	t.Cleanup(provider.Close)
	task := setupSeedanceArtifactTest(t, provider.URL)
	allowPrivateTaskMediaTest(t)
	// Delivery must still work after the current channel is removed.
	require.NoError(t, model.DB.Delete(&model.Channel{}, task.ChannelId).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", task.UserId)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/task/"+task.TaskID+"/artifacts", nil)
	GetDashboardTaskArtifacts(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Artifacts []taskArtifactResponse `json:"artifacts"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Artifacts, 1)
	artifact := response.Data.Artifacts[0]
	assert.Equal(t, "video", artifact.Key)
	assert.Equal(t, "video", artifact.Type)
	assert.NotContains(t, recorder.Body.String(), provider.URL)
	assert.NotContains(t, recorder.Body.String(), "frozen-key")
	contentURL, err := url.Parse(artifact.ContentURL)
	require.NoError(t, err)
	assert.True(t, service.VerifyTaskArtifactAccess(contentURL.Query().Get("access"), task.TaskID, "video"))

	router := gin.New()
	router.Use(middleware.TokenOrTaskArtifactAccessAuth("key", "artifact_key"))
	router.GET("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	router.HEAD("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, artifact.ContentURL, nil)
			if method == http.MethodGet {
				request.Header.Set("Range", "bytes=0-3")
			}
			router.ServeHTTP(recorder, request)
			assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			if method == http.MethodHead {
				assert.Equal(t, http.StatusOK, recorder.Code)
				assert.Empty(t, recorder.Body.String())
				assert.Equal(t, "10", recorder.Header().Get("Content-Length"))
			} else {
				assert.Equal(t, http.StatusPartialContent, recorder.Code)
				assert.Equal(t, "bytes 0-3/10", recorder.Header().Get("Content-Range"))
				assert.Equal(t, "vide", recorder.Body.String())
			}
		})
	}
	assert.Equal(t, int32(2), calls.Load())
}

func TestSeedanceArtifactProjectionUsesTaskFacts(t *testing.T) {
	for _, scenario := range []string{"pending", "failed", "missing_result", "proxy_loop", "no_plugin_snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			task := setupSeedanceArtifactTest(t, "https://media.example.invalid")
			switch scenario {
			case "pending":
				task.Status = model.TaskStatusInProgress
			case "failed":
				task.Status = model.TaskStatusFailure
			case "missing_result":
				task.PrivateData.ResultURL = ""
			case "proxy_loop":
				task.PrivateData.ResultURL = "/v1/videos/" + task.TaskID + "/content"
			case "no_plugin_snapshot":
				task.PrivateData.Execution = nil
			}
			require.NoError(t, model.DB.Save(task).Error)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", task.UserId)
			c.Set("token_id", task.AppID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts", nil)
			GetTaskArtifacts(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Artifacts []taskArtifactResponse `json:"artifacts"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			if scenario == "no_plugin_snapshot" {
				assert.Len(t, response.Artifacts, 1)
			} else {
				assert.Empty(t, response.Artifacts)
			}
		})
	}
}

func TestSeedanceArtifactAccessRejectsInvalidScopeAndUnavailableContent(t *testing.T) {
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	t.Cleanup(provider.Close)
	for _, scenario := range []string{"foreign_user", "foreign_app", "deleted", "wrong_artifact", "different_part", "expired", "missing_frozen_key", "missing_frozen_protocol"} {
		t.Run(scenario, func(t *testing.T) {
			task := setupSeedanceArtifactTest(t, provider.URL)
			allowPrivateTaskMediaTest(t)
			userID, appID, key, query := task.UserId, task.AppID, "video", ""
			expectedStatus := http.StatusNotFound
			switch scenario {
			case "foreign_user":
				userID++
			case "foreign_app":
				appID++
			case "deleted":
				task.ClientDeletedAt = common.GetTimestamp()
			case "wrong_artifact":
				key = "other"
			case "different_part":
				query = "?part=last_frame"
			case "expired":
				data, err := common.Marshal(map[string]int64{"expires_at": common.GetTimestamp() - 1})
				require.NoError(t, err)
				task.Data = data
				expectedStatus = http.StatusGone
			case "missing_frozen_key":
				task.PrivateData.Key = ""
				expectedStatus = http.StatusBadGateway
			case "missing_frozen_protocol":
				task.ClientProtocol = ""
				expectedStatus = http.StatusBadGateway
			}
			require.NoError(t, model.DB.Save(task).Error)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", userID)
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "artifact_key", Value: key}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts/"+key+"/content"+query, nil)
			TaskArtifactContent(c)
			assert.Equal(t, expectedStatus, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "frozen-key")
			if scenario == "foreign_user" || scenario == "foreign_app" || scenario == "deleted" {
				listRecorder := httptest.NewRecorder()
				listContext, _ := gin.CreateTestContext(listRecorder)
				listContext.Set("id", userID)
				listContext.Set("token_id", appID)
				listContext.Params = gin.Params{{Key: "key", Value: task.TaskID}}
				listContext.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts", nil)
				GetTaskArtifacts(listContext)
				assert.Equal(t, http.StatusNotFound, listRecorder.Code)
			}
		})
	}
	assert.Zero(t, calls.Load(), "rejected requests must never fetch media")
}

func TestSeedanceArtifactFeicaiKeepsFrozenCredentialsAndRejectsRedirects(t *testing.T) {
	var redirect atomic.Bool
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { foreignCalls.Add(1) }))
	t.Cleanup(foreign.Close)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer frozen-key", r.Header.Get("Authorization"))
		if redirect.Load() {
			http.Redirect(w, r, foreign.URL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("video"))
	}))
	t.Cleanup(provider.Close)
	task := setupSeedanceArtifactTest(t, provider.URL)
	allowPrivateTaskMediaTest(t)
	task.PrivateData.VideoUpstreamProfile = dto.VideoUpstreamProfileThirdPartyFeicaiVideos
	task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyFeicaiVideos)
	require.NoError(t, model.DB.Save(task).Error)
	for _, redirected := range []bool{false, true} {
		redirect.Store(redirected)
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", task.UserId)
		c.Set("token_id", task.AppID)
		c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "artifact_key", Value: "video"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts/video/content", nil)
		TaskArtifactContent(c)
		if redirected {
			assert.Equal(t, http.StatusBadGateway, recorder.Code)
		} else {
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "video", recorder.Body.String())
		}
		assert.Empty(t, recorder.Header().Get("Location"))
	}
	assert.Zero(t, foreignCalls.Load())
}
