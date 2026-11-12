package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/minimax"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const officialH3Hold = 2_275_000
const officialH3Final = 750_000

const officialH3Request = `{"model":"customer-video","content":[{"type":"text","text":"A landscape"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://media.example.test/reference.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://media.example.test/reference.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://media.example.test/reference.mp3"}}]}`

func newOfficialH3Fixture(t *testing.T) *seedanceFundsFixture {
	t.Helper()
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.Option{}, &model.TaskVideoResultObject{}, &model.Ability{}, &model.AuditLog{}))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr": `{"customer-video":"tier(\"official\", u(\"output_seconds\") * 0.2 + u(\"input_video_seconds\") * 0.1 + u(\"input_image_count\") * 0.05)"}`,
	}))
	fx.createBodyRes.Store(`{"task_id":"official-task-1"}`)
	fx.queryBodyRes.Store(`{"task":{"id":"official-task-1","status":"queued","model":"MiniMax-H3"}}`)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer provider-fixture-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			assert.Equal(t, "/v2/video_generation", r.URL.Path)
			fx.createCalls.Add(1)
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			fx.createBodyMu.Lock()
			fx.createBody = body
			fx.createBodyMu.Unlock()
			var attempt model.TaskCreateAttempt
			if assert.NoError(t, fx.db.First(&attempt).Error) {
				assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
				assert.Equal(t, officialH3Hold, attempt.HeldQuota)
			}
			w.WriteHeader(int(fx.createStatus.Load()))
			_, _ = io.WriteString(w, fx.createBodyRes.Load().(string))
		case http.MethodGet:
			assert.Equal(t, "/v2/query/video_generation/official-task-1", r.URL.Path)
			fx.queryCalls.Add(1)
			w.WriteHeader(int(fx.queryStatus.Load()))
			_, _ = io.WriteString(w, fx.queryBodyRes.Load().(string))
		default:
			t.Errorf("unexpected provider method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(provider.Close)
	require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Updates(map[string]any{
		"base_url": provider.URL,
		"settings": `{"video_upstream_protocol":"minimax_official_video_v2","asset_upstream_protocol":"none"}`,
	}).Error)
	return fx
}

func TestMiniMaxOfficialV2FundingPollingAndOSS(t *testing.T) {
	fx := newOfficialH3Fixture(t)
	var savedChannel model.Channel
	require.NoError(t, fx.db.First(&savedChannel, fx.channelID).Error)
	require.NoError(t, savedChannel.ValidateSettings())
	require.NoError(t, model.ValidateMiniMaxChannelModelUniqueness(fx.db, &savedChannel))
	previousFetch := *system_setting.GetFetchSetting()
	previousTLS := common.TLSInsecureSkipVerify
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	common.TLSInsecureSkipVerify = true
	service.InitHttpClient()
	t.Cleanup(func() {
		*system_setting.GetFetchSetting() = previousFetch
		common.TLSInsecureSkipVerify = previousTLS
		service.InitHttpClient()
	})

	const videoBytes = "official-h3-video-fixture"
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, videoBytes)
	}))
	t.Cleanup(cdn.Close)
	var putCount atomic.Int32
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		putCount.Add(1)
		assert.Equal(t, http.MethodPut, r.Method)
		assert.True(t, strings.HasPrefix(r.URL.Path, "/videos/videos/tasks/"))
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, videoBytes, string(body))
		assert.Equal(t, "video/mp4", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storage.Close)
	require.NoError(t, service.SaveObjectStorageSetting(system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendS3, Endpoint: storage.URL,
		Bucket: "videos", Region: "us-east-1", AccountName: "fixture-account",
		Credential: "fixture-storage-secret", Revision: "official-h3-test",
	}))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	previousResolver := service.ResolveMiniMaxVideoContentFunc
	service.ResolveMiniMaxVideoContentFunc = minimax.ResolveContentURL
	t.Cleanup(func() { service.ResolveMiniMaxVideoContentFunc = previousResolver })

	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	assert.Equal(t, "modelark_v3", catalog[0].API.Video.Protocol)
	for _, field := range catalog[0].API.Video.Creation.Parameters {
		assert.NotEqual(t, "watermark", field.Name)
	}

	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(officialH3Request))
	task := fx.loadTask(id)
	require.True(t, task.IsMiniMaxV2Task())
	assert.NotContains(t, string(task.Data), "official-task-1")
	assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Hold, fx.userQuota())
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"A landscape"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://media.example.test/reference.png"}},{"type":"video_url","role":"reference_video","video_url":{"url":"https://media.example.test/reference.mp4"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://media.example.test/reference.mp3"}}],"duration":6,"resolution":"768P","ratio":"16:9"}`, string(fx.createRequestBody()))

	var jdCreates atomic.Int32
	jdProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/task/submit", r.URL.Path)
		assert.Equal(t, "Bearer jd-fixture-key", r.Header.Get("Authorization"))
		jdCreates.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"task_id":"jd-after-switch","status":"pending"}}`)
	}))
	t.Cleanup(jdProvider.Close)
	admin := gin.New()
	admin.Use(func(c *gin.Context) {
		c.Set("id", fx.userID)
		c.Set("role", common.RoleRootUser)
	})
	admin.PUT("/api/channel/", UpdateChannel)
	admin.POST("/api/channel/:id/status", UpdateChannelStatus)
	request := func(method, path, body string) (bool, string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		admin.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var result struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
		return result.Success, result.Message
	}
	compatibleSwitch := fmt.Sprintf(`{"id":%d,"type":%d,"base_url":%q,"settings":"{\"video_upstream_protocol\":\"nexhina_hailuo_v2\"}"}`, fx.channelID, constant.ChannelTypeMiniMaxLink, savedChannel.GetBaseURL())
	successful, message := request(http.MethodPut, "/api/channel/", compatibleSwitch)
	require.True(t, successful, message)
	var stored model.Channel
	require.NoError(t, fx.db.First(&stored, fx.channelID).Error)
	assert.EqualValues(t, constant.VideoUpstreamProtocolNexhinaHailuoV2, stored.GetOtherSettings().VideoUpstreamProtocol)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)

	switchBody := fmt.Sprintf(`{"id":%d,"type":%d,"base_url":%q,"key":"jd-fixture-key","settings":"{\"video_upstream_protocol\":\"jdcloud_video_task_v1\"}"`, fx.channelID, constant.ChannelTypeMiniMaxLink, jdProvider.URL)
	successful, message = request(http.MethodPut, "/api/channel/", switchBody+"}")
	require.False(t, successful, message)
	require.Contains(t, message, "incompatible with this access method")
	require.NoError(t, fx.db.First(&stored, fx.channelID).Error)
	assert.EqualValues(t, constant.VideoUpstreamProtocolNexhinaHailuoV2, stored.GetOtherSettings().VideoUpstreamProtocol)
	assert.Equal(t, savedChannel.GetBaseURL(), stored.GetBaseURL())
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)

	successful, message = request(http.MethodPut, "/api/channel/", switchBody+`,"save_minimax_as_disabled":true}`)
	require.True(t, successful, message)
	require.NoError(t, fx.db.First(&stored, fx.channelID).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.EqualValues(t, constant.VideoUpstreamProtocolJdCloudTaskV1, stored.GetOtherSettings().VideoUpstreamProtocol)
	assert.Equal(t, savedChannel.Models, stored.Models)
	assert.Equal(t, savedChannel.GetModelMapping(), stored.GetModelMapping())
	assert.Equal(t, savedChannel.Group, stored.Group)
	enablePath := fmt.Sprintf("/api/channel/%d/status", fx.channelID)
	successful, message = request(http.MethodPost, enablePath, `{"status":1}`)
	require.False(t, successful, message)
	require.Contains(t, message, "incompatible with this access method")
	require.NoError(t, fx.db.First(&stored, fx.channelID).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)

	jdPrice := `tier("jd", u("duration_seconds") * 0.25)`
	require.NoError(t, model.ValidateModelPricing("customer-video", model.PricingValues{
		"billing_setting.billing_mode": "tiered_expr",
		"billing_setting.billing_expr": jdPrice,
	}))
	billingJSON, err := common.Marshal(map[string]string{"customer-video": jdPrice})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_expr": string(billingJSON),
	}))

	// Customer reads do not poll. The background poll uses the frozen
	// official protocol, connection and pricing while the channel is on JD.
	require.Equal(t, http.StatusOK, minimaxGet(t, fx, "/api/v3/contents/generations/tasks/"+id).Code)
	require.Equal(t, http.StatusOK, minimaxGet(t, fx, "/api/v3/contents/generations/tasks").Code)
	assert.Zero(t, fx.queryCalls.Load())
	fx.pollOnce(id)
	assert.EqualValues(t, model.TaskStatusQueued, fx.loadTask(id).Status)
	fx.queryBodyRes.Store(`{"task":{"id":"official-task-1","status":"running"}}`)
	fx.pollOnce(id)
	assert.EqualValues(t, model.TaskStatusInProgress, fx.loadTask(id).Status)
	fx.queryStatus.Store(http.StatusTooManyRequests)
	fx.pollOnce(id)
	assert.EqualValues(t, model.TaskStatusInProgress, fx.loadTask(id).Status)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Hold, fx.userQuota())
	fx.queryStatus.Store(http.StatusOK)

	success := fmt.Sprintf(`{"task":{"id":"official-task-1","status":"succeeded","model":"MiniMax-H3","task_type":"generation","modality":"video","content":{"url":%q},"resolution":"768P","ratio":"16:9","duration":6,"usage":{"total_seconds":8.5,"output_seconds":6,"input_image_count":1,"input_seconds":null,"prompt_tokens":0,"completion_tokens":42,"total_tokens":42}}}`, cdn.URL+"/video.mp4")
	fx.queryBodyRes.Store(strings.Replace(success, `"total_seconds":8.5`, `"total_seconds":2`, 1))
	fx.pollOnce(id)
	task = fx.loadTask(id)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, task.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Hold, fx.userQuota())
	assert.NotContains(t, string(task.Data), cdn.URL)
	assert.NotContains(t, string(task.Data), "official-task-1")
	assert.NotContains(t, string(task.Data), "official_usage")
	assert.NotContains(t, string(task.Data), "_modelark_display_usage")
	assert.Zero(t, task.ToModelArkVideoTask().Usage.TotalSeconds)
	require.NotNil(t, task.PrivateData.ModelArkUsage)
	assert.Nil(t, task.PrivateData.ModelArkUsage.TotalSeconds)
	legacy := task
	legacyDisplay := *task.PrivateData.ModelArkUsage
	legacyDisplay.TotalSeconds = common.GetPointer(2.0)
	legacy.PrivateData.ModelArkUsage = &legacyDisplay
	assert.Zero(t, legacy.ToModelArkVideoTask().Usage.TotalSeconds)
	fx.queryBodyRes.Store(strings.Replace(success, `"total_seconds":8.5`, `"total_seconds":30`, 1))
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	assert.Zero(t, task.ToModelArkVideoTask().Usage.TotalSeconds)
	fx.queryBodyRes.Store(success)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	assert.Equal(t, 8.5, task.ToModelArkVideoTask().Usage.TotalSeconds)
	assert.Equal(t, http.StatusConflict, minimaxGet(t, fx, "/v1/videos/"+id+"/content").Code)

	// The same transfer worker stores the real fixture bytes through the S3
	// adapter while billing still waits for measured input video seconds.
	assert.Equal(t, 1, service.RunVideoResultTransferPass(context.Background(), nil).Ready)
	assert.EqualValues(t, 1, putCount.Load())
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, fx.loadTask(id).BillingState)
	w := minimaxGet(t, fx, "/v1/videos/"+id+"/content")
	require.Equal(t, http.StatusFound, w.Code)
	location, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, strings.TrimPrefix(storage.URL, "http://"), location.Host)
	assert.Equal(t, "86400", location.Query().Get("X-Amz-Expires"))

	// Later observation fills the missing meter but cannot replace an already
	// accepted amount. The existing recovery job completes the same funding.
	complete := strings.Replace(success, `"input_seconds":null,"prompt_tokens":0`, `"input_seconds":2.5,"input_audio_seconds":6.25,"prompt_tokens":99`, 1)
	complete = strings.Replace(complete, `"output_seconds":6`, `"output_seconds":7`, 1)
	fx.queryBodyRes.Store(complete)
	service.ReconcileTaskUsage(context.Background())
	task = fx.loadTask(id)
	require.Equal(t, model.TaskBillingStateSettled, task.BillingState)
	assert.Equal(t, map[string]float64{"output_seconds": 6, "input_video_seconds": 2.5, "input_image_count": 1}, task.PrivateData.AsyncBilling.OfficialUsage)
	assert.Equal(t, map[string]float64{"output_seconds": 7}, task.PrivateData.AsyncBilling.OfficialUsageConflict)
	assert.Equal(t, officialH3Final, task.Quota)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Final, fx.userQuota())
	remain, used := fx.tokenQuota()
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Final, remain)
	assert.Equal(t, officialH3Final, used)
	logs := fx.logsByType()
	require.Len(t, logs[model.LogTypeConsume], 1)
	require.Len(t, logs[model.LogTypeRefund], 1)
	assert.Equal(t, officialH3Hold, logs[model.LogTypeConsume][0].Quota)
	assert.Equal(t, officialH3Hold-officialH3Final, logs[model.LogTypeRefund][0].Quota)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Final, fx.userQuota())
	var deliveries []model.TaskBillingDelivery
	require.NoError(t, fx.db.Where("task_row_id = ?", task.ID).Find(&deliveries).Error)
	assert.Len(t, deliveries, 2)
	assert.EqualValues(t, 1, fx.createCalls.Load())

	before := fx.queryCalls.Load()
	w = minimaxGet(t, fx, "/api/v3/contents/generations/tasks/"+id)
	require.Equal(t, http.StatusOK, w.Code)
	var public dto.ModelArkVideoTask
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &public))
	require.NotNil(t, public.Content)
	assert.Equal(t, "ready", public.Content.DeliveryStatus)
	assert.Equal(t, "customer-video", public.Model)
	expectedUsage := dto.ModelArkVideoTaskUsage{
		TotalSeconds: 8.5, InputSeconds: 2.5, OutputSeconds: 6, InputAudioSeconds: 6.25,
		InputImageCount: 1, CompletionTokens: 42, TotalTokens: 42,
	}
	assert.Equal(t, expectedUsage, public.Usage)
	assert.NotContains(t, w.Body.String(), "_modelark_display_usage")
	assert.NotContains(t, w.Body.String(), "official-task-1")
	assert.NotContains(t, w.Body.String(), cdn.URL)
	w = minimaxGet(t, fx, "/api/v3/contents/generations/tasks")
	require.Equal(t, http.StatusOK, w.Code)
	var list dto.ModelArkVideoTaskList
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, public.Usage, list.Items[0].Usage)
	assert.Equal(t, before, fx.queryCalls.Load())

	successful, message = request(http.MethodPost, enablePath, `{"status":1}`)
	require.True(t, successful, message)
	newID := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`))
	newTask := fx.loadTask(newID)
	assert.EqualValues(t, constant.VideoUpstreamProtocolJdCloudTaskV1, newTask.PrivateData.VideoUpstreamProtocol)
	assert.Equal(t, minimaxFundsHold, newTask.Quota)
	assert.EqualValues(t, 1, jdCreates.Load())
	assert.EqualValues(t, 1, fx.createCalls.Load())
}

func TestMiniMaxVerifiedMediaReviewSettlesFrozenUsageOnce(t *testing.T) {
	fx := newOfficialH3Fixture(t)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(officialH3Request))
	fx.queryBodyRes.Store(`{"task":{"id":"official-task-1","status":"succeeded","content":{"url":"https://example.test/clip.mp4"},"usage":{"output_seconds":6}}}`)
	fx.pollOnce(id)
	require.Equal(t, model.TaskBillingStateAwaitingUsage, fx.loadTask(id).BillingState)
	for _, invalid := range []map[string]*float64{
		{"input_image_count": nil},
		{"input_image_count": common.GetPointer(2.0)},
		{"output_seconds": common.GetPointer(7.0)},
		{"completion_tokens": common.GetPointer(0.0)},
	} {
		_, err := model.ReviewTaskUsage(id, 9, "statement-invalid", nil, invalid)
		require.Error(t, err)
	}
	image := map[string]*float64{"input_image_count": common.GetPointer(1.0)}
	reviewed, err := model.ReviewTaskUsage(id, 9, "statement-image", nil, image)
	require.NoError(t, err)
	assert.Equal(t, model.TaskBillingStateAwaitingUsage, reviewed.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Hold, fx.userQuota())
	video := map[string]*float64{"input_video_seconds": common.GetPointer(2.5)}
	reviewed, err = model.ReviewTaskUsage(id, 9, "statement-video", nil, video)
	require.NoError(t, err)
	assert.Equal(t, model.TaskBillingStatePending, reviewed.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Hold, fx.userQuota())
	service.ReconcileTaskBilling(context.Background(), 10)
	settled := fx.loadTask(id)
	require.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
	assert.Equal(t, officialH3Final, settled.Quota)
	assert.Equal(t, map[string]string{"input_image_count": "statement-image", "input_video_seconds": "statement-video"}, settled.PrivateData.AsyncBilling.OfficialUsageStatements)
	_, err = model.ReviewTaskUsage(id, 9, "statement-replay", nil, video)
	require.NoError(t, err)
	_, err = model.ReviewTaskUsage(id, 9, "statement-conflict", nil, map[string]*float64{"input_video_seconds": common.GetPointer(3.0)})
	require.ErrorContains(t, err, "cannot be replaced")
	service.ReconcileTaskBilling(context.Background(), 10)
	assert.Equal(t, seedanceFundsInitialQuota-officialH3Final, fx.userQuota())
	assert.Len(t, fx.logsByType()[model.LogTypeRefund], 1)
}

func TestMiniMaxOfficialCreateRejectsOrPreservesHold(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		hold   model.TaskCreateAttemptBillingHoldState
	}{
		{"confirmed refusal", 400, `{"type":"error","error":{"type":"bad_request_error","message":"invalid input","http_code":"400"}}`, model.TaskCreateAttemptBillingReleased},
		{"contradictory refusal", 400, `{"type":"error","task_id":"official-task-1","error":{"type":"bad_request_error","message":"invalid input","http_code":"400"}}`, model.TaskCreateAttemptBillingHeld},
		{"missing accepted id", 200, `{}`, model.TaskCreateAttemptBillingHeld},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newOfficialH3Fixture(t)
			fx.createStatus.Store(int32(tc.status))
			fx.createBodyRes.Store(tc.body)
			fx.submitCreateBody(officialH3Request)
			var attempt model.TaskCreateAttempt
			require.NoError(t, fx.db.First(&attempt).Error)
			assert.Equal(t, tc.hold, attempt.BillingHoldState)
			var count int64
			require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
			assert.Zero(t, count)
			assert.EqualValues(t, 1, fx.createCalls.Load())
		})
	}
}
