package controller

import (
	"bytes"
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedVideoCreationPersistsFactsAndRejectsBeforeHold(t *testing.T) {
	for _, outcome := range []string{"accepted", "delete_after_accept", "ambiguous", "other_user", "deleted", "missing", "video_slot", "audio_slot", "non_hosted", "object_missing", "store_unavailable", "location_changed", "direct_url"} {
		t.Run(outcome, func(t *testing.T) { testHostedVideoCreation(t, dto.VideoUpstreamProtocolFunCloudModelArkV3, outcome) })
	}
}

func TestSynlinkHostedVideoDurableLifecycle(t *testing.T) {
	for _, outcome := range []string{"accepted", "delete_after_accept", "ambiguous", "direct_url", "other_user", "object_missing", "non_hosted", "polling_lifecycle"} {
		t.Run(outcome, func(t *testing.T) { testHostedVideoCreation(t, dto.VideoUpstreamProtocolSynlinkVideoV1, outcome) })
	}
}

func testHostedVideoCreation(t *testing.T, protocol dto.VideoUpstreamProtocol, outcome string) {
	t.Helper()
	service.InitHttpClient()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	events := []string{}
	db := setupTaskSubmissionDatabase(t, true, &events)
	seedPublishedSeedanceControllerArtifact(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.TaskCreateAttempt{}, &model.TaskCreateIdempotency{}, &model.UserSubscription{}, &model.FunCloudHostedAsset{}, &model.FunCloudHostedAssetGroup{}))
	oldLog, oldCache, oldRedis, oldBatch, oldConsume := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		model.LOG_DB = oldLog
		common.MemoryCacheEnabled = oldCache
		common.RedisEnabled = oldRedis
		common.BatchUpdateEnabled = oldBatch
		common.LogConsumeEnabled = oldConsume
	})
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"customer-funcloud":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"customer-funcloud":"tier(\"fixed\", 0.0014)"}`,
	}))
	if outcome == "polling_lifecycle" {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"billing_setting.billing_expr":           `{"customer-funcloud":"tier(\"tokens\", u(\"tokens\") * 2 / 1000000)"}`,
			"task_billing_setting.preconsume_tokens": `{"customer-funcloud":700}`,
		}))
	}
	user := model.User{Id: 8190, Username: "funcloud-integration", Quota: 10000, Status: common.UserStatusEnabled, Group: "default"}
	user.SetSetting(kitdto.UserSetting{BillingPreference: "wallet_only"})
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{Id: 8190, UserId: user.Id, Key: strings.Repeat("f", 32), Status: common.TokenStatusEnabled, RemainQuota: 10000, ExpiredTime: -1, Group: "default"}
	require.NoError(t, db.Create(&token).Error)

	var providerCalls, storageCalls, pollStage atomic.Int32
	objectStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageCalls.Add(1)
		assert.Equal(t, http.MethodHead, r.Method)
		assert.Equal(t, "/images/prod/assets/hosted.png", r.URL.Path)
		if outcome == "object_missing" {
			w.WriteHeader(404)
			return
		}
		if outcome == "store_unavailable" {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Length", "12")
	}))
	t.Cleanup(objectStore.Close)
	storageConfig := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: objectStore.URL, Bucket: "images", Prefix: "prod", AccountName: "account", Region: "us-east-1", Credential: "fixture-secret", Revision: outcome}
	encoded, err := common.Marshal(storageConfig)
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(encoded))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	asset := model.FunCloudHostedAsset{ID: "fhas_fixture", UserID: user.Id, Name: "fixture", Model: "customer-funcloud", ObjectKey: "assets/hosted.png", MimeType: "image/png", SizeBytes: 12, Status: model.FunCloudHostedAssetStatusReady,
		StorageLocation: model.FunCloudHostedStorageLocation{Backend: "s3", Endpoint: objectStore.URL, Bucket: "images", Prefix: "prod"}}
	if outcome == "other_user" {
		asset.UserID++
	}
	if outcome == "deleted" {
		asset.Status = model.FunCloudHostedAssetStatusDeleted
	}
	if outcome == "location_changed" {
		asset.StorageLocation.Bucket = "old-bucket"
	}
	if outcome != "missing" {
		require.NoError(t, db.Create(&asset).Error)
	}
	var sendingFacts []model.TaskHostedMediaFact
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			assert.Equal(t, "/v1/video/tasks/hosted-provider-task", r.URL.Path)
			assert.Equal(t, "Bearer provider-fixture-key", r.Header.Get("Authorization"))
			stage := int(pollStage.Load())
			if stage >= 400 {
				w.WriteHeader(stage)
				_, _ = io.WriteString(w, `<html>unverified response</html>`)
				return
			}
			switch stage {
			case 1:
				_, _ = io.WriteString(w, `{"task":{"id":"hosted-provider-task","status":"processing"}}`)
			case 2:
				_, _ = io.WriteString(w, `{"task":{"id":"hosted-provider-task","status":"pending"}}`)
			case 3:
				_, _ = io.WriteString(w, `{"task":{"id":"hosted-provider-task","status":"completed","outputs":["https://cdn.example/video.mp4"]}}`)
			default:
				_, _ = io.WriteString(w, `{"task":{"id":"hosted-provider-task","status":"completed","outputs":["https://cdn.example/video.mp4"],"usage":{"completion_tokens":100,"total_tokens":100}}}`)
			}
			return
		}
		providerCalls.Add(1)
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			assert.Equal(t, "/v1/video/generate", r.URL.Path)
		}
		var attempt model.TaskCreateAttempt
		if !assert.NoError(t, db.First(&attempt).Error) {
			w.WriteHeader(500)
			return
		}
		assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
		assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
		assert.Equal(t, 700, attempt.HeldQuota)
		var tasksBeforeAcceptance int64
		assert.NoError(t, db.Model(&model.Task{}).Count(&tasksBeforeAcceptance).Error)
		assert.Zero(t, tasksBeforeAcceptance, "a Task requires a trusted Provider ID")
		var snapshot struct {
			PrivateData model.TaskPrivateData `json:"private_data"`
		}
		assert.NoError(t, common.Unmarshal(attempt.RecoverySnapshot, &snapshot))
		sendingFacts = snapshot.PrivateData.HostedMedia
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		var payload struct {
			Content []struct {
				Type     string `json:"type"`
				Role     string `json:"role"`
				ImageURL *struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"content"`
			RealPersonMode bool `json:"real_person_mode"`
		}
		if !assert.NoError(t, common.Unmarshal(body, &payload)) {
			w.WriteHeader(500)
			return
		}
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			assert.NotContains(t, string(body), "real_person_mode")
			assert.NotContains(t, string(body), "omni_reference_task_type")
			assert.NotContains(t, string(body), "asset://")
			assert.Equal(t, "https://source.example/direct.png?one=1&two=2", payload.Content[2].ImageURL.URL)
			assert.Equal(t, "image_url", payload.Content[2].Type)
			assert.Equal(t, "reference_image", payload.Content[2].Role)
			require.NotNil(t, snapshot.PrivateData.ClientRequest)
			assert.Equal(t, common.GetPointer(true), snapshot.PrivateData.ClientRequest.ReturnLastFrame)
		} else {
			assert.True(t, payload.RealPersonMode)
		}
		assert.NotContains(t, string(body), "asset://fhas_")
		if outcome == "direct_url" {
			assert.Empty(t, sendingFacts)
			assert.Contains(t, string(body), "https://source.example/image.png")
		} else {
			if assert.Len(t, sendingFacts, 1) {
				assert.Equal(t, asset.StorageLocation, sendingFacts[0].StorageLocation)
				assert.Equal(t, asset.ObjectKey, sendingFacts[0].ObjectKey)
			}
			signed, err := url.Parse(payload.Content[1].ImageURL.URL)
			assert.NoError(t, err)
			assert.Equal(t, "/images/prod/assets/hosted.png", signed.Path)
			assert.Equal(t, "86400", signed.Query().Get("X-Amz-Expires"))
		}
		if outcome == "delete_after_accept" {
			_, err := model.MarkFunCloudHostedAssetDeleted(user.Id, asset.ID)
			assert.NoError(t, err)
		}
		w.Header().Set("Content-Type", "application/json")
		if outcome == "ambiguous" {
			_, _ = w.Write([]byte(`{"unexpected":true}`))
			return
		}
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			_, _ = io.WriteString(w, `{"task":{"id":"hosted-provider-task","status":"pending","outputs":[]}}`)
		} else {
			_, _ = w.Write([]byte(`{"id":"hosted-provider-task"}`))
		}
	}))
	t.Cleanup(provider.Close)
	channel := model.Channel{Id: 8190, Type: constant.ChannelTypeSeedanceLink, Name: "fixture", Key: "provider-fixture-key", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, Models: "customer-funcloud", Group: "default", ModelMapping: common.GetPointer(`{"customer-funcloud":"seedance-2-0"}`)}
	if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
		channel.ModelMapping = common.GetPointer(`{"customer-funcloud":"doubao-seedance-2-0-260128"}`)
	}
	settings := dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted}
	if outcome == "non_hosted" {
		settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
	}
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(&channel).Error)
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(), middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
	mediaType, role, reference := "image_url", "reference_image", "asset://fhas_fixture"
	if outcome == "video_slot" {
		mediaType, role = "video_url", "reference_video"
	}
	if outcome == "audio_slot" {
		mediaType, role = "audio_url", "reference_audio"
	}
	if outcome == "direct_url" {
		reference = "https://source.example/image.png"
	}
	payload := map[string]any{"model": "customer-funcloud", "duration": 4, "content": []any{map[string]any{"type": "text", "text": "Landscape"}, map[string]any{"type": mediaType, "role": role, mediaType: map[string]any{"url": reference}}}}
	if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
		payload["return_last_frame"] = true
		payload["content"] = append(payload["content"].([]any), map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "https://source.example/direct.png?one=1&two=2"}})
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var tasks []model.Task
	require.NoError(t, db.Find(&tasks).Error)
	var attempts []model.TaskCreateAttempt
	require.NoError(t, db.Find(&attempts).Error)
	var finalUser model.User
	require.NoError(t, db.First(&finalUser, user.Id).Error)
	if outcome == "accepted" || outcome == "delete_after_accept" || outcome == "direct_url" || outcome == "polling_lifecycle" {
		require.Equal(t, 200, response.Code, response.Body.String())
		require.Equal(t, int32(1), providerCalls.Load())
		require.Len(t, tasks, 1)
		assert.Equal(t, sendingFacts, tasks[0].PrivateData.HostedMedia)
		assert.Equal(t, 9300, finalUser.Quota)
		require.Len(t, attempts, 1)
		assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempts[0].BillingHoldState)
		assert.Equal(t, model.TaskCreateAttemptComplete, attempts[0].Status)
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			assert.Equal(t, common.GetPointer(true), tasks[0].PrivateData.ClientRequest.ReturnLastFrame)
		}
	} else if outcome == "ambiguous" {
		require.Equal(t, int32(1), providerCalls.Load())
		require.Empty(t, tasks)
		require.Len(t, attempts, 1)
		assert.Equal(t, model.TaskCreateAttemptUnknown, attempts[0].Status)
		assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempts[0].BillingHoldState)
		assert.Equal(t, 9300, finalUser.Quota)
		assert.NotContains(t, string(attempts[0].RecoverySnapshot), "X-Amz-")
		if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			recovered, err := service.RecoverUnknownTaskCreateAttempt(attempts[0].AttemptID, "hosted-provider-task", "", true, user.Id, "provider verified")
			require.NoError(t, err)
			require.NoError(t, db.Where("task_id = ?", recovered.PublicTaskID).Find(&tasks).Error)
			require.Len(t, tasks, 1)
			assert.Equal(t, sendingFacts, tasks[0].PrivateData.HostedMedia)
			assert.Equal(t, common.GetPointer(true), tasks[0].PrivateData.ClientRequest.ReturnLastFrame)
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			assert.Equal(t, 9300, finalUser.Quota)
			assert.Equal(t, int32(1), providerCalls.Load(), "manual recovery must not resend")
			require.NoError(t, db.First(&attempts[0], attempts[0].ID).Error)
			assert.Equal(t, model.TaskCreateAttemptComplete, attempts[0].Status)
			assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempts[0].BillingHoldState)
			assert.Empty(t, attempts[0].RecoverySnapshot)
		}
	} else {
		if outcome == "object_missing" || outcome == "store_unavailable" || outcome == "location_changed" {
			assert.Equal(t, 503, response.Code, response.Body.String())
		} else {
			assert.Equal(t, 400, response.Code, response.Body.String())
		}
		assert.Zero(t, providerCalls.Load())
		assert.Empty(t, tasks)
		assert.Empty(t, attempts)
		assert.Equal(t, 10000, finalUser.Quota)
		var finalToken model.Token
		require.NoError(t, db.First(&finalToken, token.Id).Error)
		assert.Equal(t, 10000, finalToken.RemainQuota)
	}
	if outcome == "direct_url" {
		assert.Zero(t, storageCalls.Load())
	}
	assert.NotContains(t, response.Body.String(), "X-Amz-")
	assert.NotContains(t, response.Body.String(), "assets/hosted.png")
	if len(tasks) > 0 {
		encoded, err := common.Marshal(tasks[0].PrivateData)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "X-Amz-")
	}
	if outcome == "polling_lifecycle" {
		oldFactory := service.GetTaskAdaptorFunc
		service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &seedance.TaskAdaptor{} }
		t.Cleanup(func() { service.GetTaskAdaptorFunc = oldFactory })
		task := tasks[0]
		var startedAt int64
		for _, stage := range []int{404, 410, 401, 403, 429, 500, 2, 1, 1, 3, 404, 1, 4} {
			pollStage.Store(int32(stage))
			err := service.RefreshVideoTask(context.Background(), &task)
			if (stage == 404 || stage == 1) && task.Status == model.TaskStatusSuccess {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, db.First(&task, task.ID).Error)
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			switch {
			case stage == 4:
				assert.Equal(t, model.TaskBillingStateSettled, task.BillingState)
				assert.Equal(t, 100, task.Quota)
				assert.Equal(t, 9900, finalUser.Quota)
			case stage == 3 || task.Status == model.TaskStatusSuccess:
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), task.Status)
				publicTask := task.ToModelArkVideoTask()
				require.NotNil(t, publicTask.Content)
				assert.Equal(t, "/v1/videos/"+task.TaskID+"/content?part=last_frame", publicTask.Content.LastFrameURL)
				assert.Equal(t, model.TaskBillingStateAwaitingUsage, task.BillingState)
				assert.Equal(t, 700, task.Quota)
				assert.Equal(t, 9300, finalUser.Quota)
			default:
				expected := model.TaskStatusReconciliationRequired
				if stage == 2 {
					expected = model.TaskStatusQueued
				} else if stage == 1 {
					expected = model.TaskStatusInProgress
					require.NotZero(t, task.StartTime)
					if startedAt == 0 {
						startedAt = task.StartTime
					}
					assert.Equal(t, startedAt, task.StartTime)
					assert.Empty(t, task.FailReason)
					assert.Empty(t, task.PrivateData.ResultURL)
					assert.False(t, task.PrivateData.AsyncBilling.ActualUsageReported)
				}
				require.Equal(t, expected, task.Status)
				assert.Equal(t, 700, task.Quota)
				assert.Equal(t, 9300, finalUser.Quota)
			}
		}
		require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
		require.NoError(t, db.First(&finalUser, user.Id).Error)
		assert.Equal(t, 9900, finalUser.Quota, "repeat polling must not settle twice")
		assert.Equal(t, int32(1), providerCalls.Load())
	}

}
