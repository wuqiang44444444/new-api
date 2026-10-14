package controller

import (
	"context"
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

func setupSeedanceArtifactStorage(t *testing.T) model.VideoResultStorageLocation {
	t.Helper()
	previousMain := common.MainDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() { common.SetDatabaseTypes(previousMain, previousMain) })
	config := system_setting.ObjectStorageConfig{
		Backend:     system_setting.ObjectStorageBackendS3,
		Endpoint:    "http://oss.example.test",
		Bucket:      "video-bucket",
		Region:      "us-east-1",
		AccountName: "test-key",
		Credential:  "test-secret",
		Revision:    common.GetUUID(),
	}
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.SaveObjectStorageSetting(string(raw)))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	require.True(t, service.VideoResultDeliveryAvailable(context.Background()))
	return model.VideoResultStorageLocation{Backend: "s3", Endpoint: config.Endpoint, Bucket: config.Bucket}
}

func insertSeedanceHostedRow(t *testing.T, task *model.Task, part, objectKey, mimeType string) {
	t.Helper()
	now := common.GetTimestamp()
	row := model.TaskVideoResultObject{
		TaskID: task.TaskID, Part: part, UserID: task.UserId, AppID: task.AppID,
		Status:    model.TaskVideoResultStatusReady,
		Location:  model.VideoResultStorageLocation{Backend: "s3", Endpoint: "http://oss.example.test", Bucket: "video-bucket"},
		ObjectKey: objectKey, MimeType: mimeType, SizeBytes: 10,
		CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&row).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("task_id = ?", task.TaskID).Delete(&model.TaskVideoResultObject{}).Error
	})
}

// 托管交付：控制台制品授权后重定向本站预签名地址；渠道删除不影响交付，
// 网关不再向上游回源传输字节（方案 §3.4）。
func TestSeedanceDashboardArtifactsDeliverHostedVideo(t *testing.T) {
	task := setupSeedanceArtifactTest(t, "https://media.example.invalid")
	setupSeedanceArtifactStorage(t)
	task.FinishTime = common.GetTimestamp()
	require.NoError(t, model.DB.Save(task).Error)
	insertSeedanceHostedRow(t, task, model.TaskVideoResultPartVideo, "videos/tasks/"+task.TaskID+"/video", "video/mp4")
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
	assert.NotContains(t, recorder.Body.String(), "media.example.invalid")
	contentURL, err := url.Parse(artifact.ContentURL)
	require.NoError(t, err)
	assert.True(t, service.VerifyTaskArtifactAccess(contentURL.Query().Get("access"), task.TaskID, "video"))

	router := gin.New()
	router.Use(middleware.TokenOrTaskArtifactAccessAuth("key", "artifact_key"))
	router.GET("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, artifact.ContentURL, nil))
	assert.Equal(t, http.StatusFound, recorder.Code)
	location := recorder.Header().Get("Location")
	assert.Contains(t, location, "video-bucket")
	assert.Contains(t, location, "videos/tasks/"+task.TaskID+"/video")
	assert.Contains(t, location, "X-Amz-Signature")
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
	now := common.GetTimestamp()
	for _, scenario := range []string{"foreign_user", "foreign_app", "deleted", "wrong_artifact", "delivery_preparing", "delivery_unavailable", "stock_outside_window", "missing_frozen_protocol"} {
		t.Run(scenario, func(t *testing.T) {
			task := setupSeedanceArtifactTest(t, provider.URL)
			setupSeedanceArtifactStorage(t)
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
			case "delivery_preparing":
				task.FinishTime = now
				expectedStatus = http.StatusConflict
			case "delivery_unavailable":
				task.FinishTime = now
				row := model.TaskVideoResultObject{
					TaskID: task.TaskID, Part: model.TaskVideoResultPartVideo,
					UserID: task.UserId, AppID: task.AppID,
					Status: model.TaskVideoResultStatusUnavailable, LastError: "upstream result expired",
					CreatedAt: now, UpdatedAt: now,
				}
				require.NoError(t, model.DB.Create(&row).Error)
				expectedStatus = http.StatusGone
			case "stock_outside_window":
				expectedStatus = http.StatusGone
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
			assert.NotContains(t, recorder.Body.String(), "media.example", "upstream direct links must never surface")
			if scenario == "delivery_preparing" {
				assert.Contains(t, recorder.Body.String(), "video_delivery_preparing")
			}
			if scenario == "delivery_unavailable" || scenario == "stock_outside_window" {
				assert.Contains(t, recorder.Body.String(), "video_delivery_unavailable")
			}
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
