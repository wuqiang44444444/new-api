package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 托管交付：控制台制品授权后重定向本站预签名地址；渠道删除、查询与计费
// 状态不受影响，客户端路径不再回源 Provider，也不泄露上游地址与凭据。
func TestMiniMaxDashboardArtifactsDeliverHostedVideo(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.Option{}, &model.TaskVideoResultObject{}))
	setupSeedanceArtifactStorage(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "minimax-artifact-fixture"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`))
	task := fx.loadTask(id)
	fx.queryBodyRes.Store(minimaxSuccess)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	require.EqualValues(t, model.TaskStatusSuccess, fx.loadTask(id).Status)
	quota := fx.userQuota()
	// 成功即登记待转存；worker 语义由 service 测试覆盖，这里把行推进到 ready。
	rows, err := model.ListVideoResultObjects(id)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	claimed, err := model.ClaimVideoResultObjects(5, "artifact-runner", common.GetTimestamp(), 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	won, err := model.FinishVideoResultObjectReady(claimed[0].ID, "artifact-runner",
		model.VideoResultStorageLocation{Backend: "s3", Endpoint: "http://oss.example.test", Bucket: "video-bucket"},
		"videos/tasks/"+id+"/video", "video/mp4", 10, common.GetTimestamp())
	require.NoError(t, err)
	require.True(t, won)
	// The stored object is the only delivery fact; the channel row is gone.
	require.NoError(t, fx.db.Delete(&model.Channel{}, fx.channelID).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", fx.userID)
	c.Params = gin.Params{{Key: "task_id", Value: id}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/task/"+id+"/artifacts", nil)
	GetDashboardTaskArtifacts(c)
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Data struct {
			Artifacts []taskArtifactResponse `json:"artifacts"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data.Artifacts, 1)
	artifact := response.Data.Artifacts[0]
	assert.Equal(t, "video", artifact.Key)
	assert.Equal(t, "video/mp4", artifact.MimeType)
	assert.NotContains(t, w.Body.String(), "8.8.8.8")
	assert.NotContains(t, w.Body.String(), "provider-fixture-key")
	contentURL, err := url.Parse(artifact.ContentURL)
	require.NoError(t, err)
	require.True(t, service.VerifyTaskArtifactAccess(contentURL.Query().Get("access"), id, "video"))
	router := gin.New()
	router.Use(middleware.TokenOrTaskArtifactAccessAuth("key", "artifact_key"))
	router.GET("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	router.HEAD("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	before := fx.queryCalls.Load()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, artifact.ContentURL, nil)
		router.ServeHTTP(w, r)
		assert.Equal(t, http.StatusFound, w.Code, "hosted delivery must redirect, not stream")
		assert.Contains(t, w.Header().Get("Location"), "videos/tasks/"+id+"/video")
		assert.NotContains(t, w.Header().Get("Location"), "8.8.8.8")
	}
	assert.Equal(t, before, fx.queryCalls.Load(), "client delivery must not query the provider")
	assert.EqualValues(t, 1, fx.createCalls.Load())
	assert.Equal(t, quota, fx.userQuota(), "artifact delivery never charges again")
	assert.EqualValues(t, model.TaskStatusSuccess, fx.loadTask(id).Status)
	assert.Equal(t, model.TaskBillingStateSettled, fx.loadTask(id).BillingState)
}

// 客户端内容路径零回源：即使登记的上游地址是私网地址，交付也只重定向本站
// 签名地址；SSRF 边界由转存 worker 在读取上游时执行（见 service 测试）。
func TestMiniMaxArtifactContentDeliveryDoesNotFetchUpstream(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.Option{}, &model.TaskVideoResultObject{}))
	setupSeedanceArtifactStorage(t)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`))
	task := fx.loadTask(id)
	fx.queryBodyRes.Store(strings.Replace(minimaxSuccess, "8.8.8.8", "127.0.0.1", 1))
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	previous := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = false
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
	claimed, err := model.ClaimVideoResultObjects(5, "artifact-runner", common.GetTimestamp(), 1800)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	won, err := model.FinishVideoResultObjectReady(claimed[0].ID, "artifact-runner",
		model.VideoResultStorageLocation{Backend: "s3", Endpoint: "http://oss.example.test", Bucket: "video-bucket"},
		"videos/tasks/"+id+"/video", "video/mp4", 10, common.GetTimestamp())
	require.NoError(t, err)
	require.True(t, won)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", task.UserId)
	c.Set("token_id", task.AppID)
	c.Params = gin.Params{{Key: "key", Value: id}, {Key: "artifact_key", Value: "video"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+id+"/artifacts/video/content", nil)
	TaskArtifactContent(c)
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Contains(t, w.Header().Get("Location"), "video-bucket")
	assert.NotContains(t, w.Header().Get("Location"), "127.0.0.1")
}

func TestMiniMaxArtifactsRejectUnavailableAndForeignResources(t *testing.T) {
	for _, scenario := range []string{"pending", "failed", "deleted", "foreign_user", "foreign_app", "missing_upstream_id", "wrong_artifact", "different_part", "missing_frozen_key", "missing_plugin", "delivery_unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			task := setupSeedanceArtifactTest(t, "https://example.invalid")
			task.Platform = "69"
			task.PrivateData.VideoUpstreamProtocol = "jdcloud_video_task_v1"
			task.PrivateData.ResultURL = "/v1/videos/" + task.TaskID + "/content"
			task.PrivateData.Execution.TaskPlugin.Key = "minimax-link"
			userID, appID, artifact, query := task.UserId, task.AppID, "video", ""
			status := http.StatusNotFound
			switch scenario {
			case "pending":
				task.Status = model.TaskStatusInProgress
				status = http.StatusConflict
			case "failed":
				task.Status = model.TaskStatusFailure
				status = http.StatusConflict
			case "deleted":
				task.ClientDeletedAt = 1
			case "foreign_user":
				userID++
			case "foreign_app":
				appID++
			case "missing_upstream_id":
				task.PrivateData.UpstreamTaskID = ""
			case "wrong_artifact":
				artifact = "other"
			case "different_part":
				query = "?part=last_frame"
			case "missing_frozen_key":
				// 冻结凭据只影响后台转存；客户端交付按登记事实表达。
				status = http.StatusGone
			case "missing_plugin":
				task.PrivateData.Execution = nil
				status = http.StatusGone
			case "delivery_unavailable":
				row := model.TaskVideoResultObject{
					TaskID: task.TaskID, Part: model.TaskVideoResultPartVideo,
					UserID: task.UserId, AppID: task.AppID,
					Status: model.TaskVideoResultStatusUnavailable, LastError: "upstream result expired",
					CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp(),
				}
				require.NoError(t, model.DB.Create(&row).Error)
				status = http.StatusGone
			}
			require.NoError(t, model.DB.Save(task).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", userID)
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "artifact_key", Value: artifact}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts/"+artifact+"/content"+query, nil)
			TaskArtifactContent(c)
			assert.Equal(t, status, w.Code, w.Body.String())
			if scenario == "wrong_artifact" || scenario == "different_part" || scenario == "missing_frozen_key" || scenario == "missing_plugin" || scenario == "delivery_unavailable" {
				return
			}
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Set("id", userID)
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts", nil)
			GetTaskArtifacts(c)
			if scenario == "deleted" || scenario == "foreign_user" || scenario == "foreign_app" {
				assert.Equal(t, http.StatusNotFound, w.Code)
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				var body struct {
					Artifacts []taskArtifactResponse `json:"artifacts"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
				assert.Empty(t, body.Artifacts)
			}
		})
	}
}
