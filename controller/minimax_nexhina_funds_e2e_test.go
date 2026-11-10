package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxNexhinaConfiguredModelFunding(t *testing.T) {
	const hold = 600_000
	const final = 160_000
	const providerModel = "MiniMax-H3-idc@revision:blue"
	require.NoError(t, i18n.Init())
	fx := newMiniMaxFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.Option{}, &model.TaskVideoResultObject{}))
	require.NoError(t, fx.db.Model(&model.Token{}).Where("id = ?", fx.userID).Update("remain_quota", hold).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"customer-video":"tiered_expr","customer-video-idc":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"customer-video":"tier(\"jd\", u(\"duration_seconds\") * 0.25)","customer-video-idc":"tier(\"nexhina\", u(\"output_seconds\") * 0.08)"}`,
	}))
	fx.queryBodyRes.Store(`{"task":{"id":"nexhina-idc-task","status":"queued"}}`)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			fx.createCalls.Add(1)
			assert.Equal(t, "/hailuo/v2/video_generation", r.URL.Path)
			var request map[string]any
			assert.NoError(t, common.DecodeJson(r.Body, &request))
			assert.Equal(t, providerModel, request["model"])
			assert.Equal(t, "768P", request["resolution"])
			_, _ = io.WriteString(w, `{"task_id":"nexhina-idc-task"}`)
		case http.MethodGet:
			fx.queryCalls.Add(1)
			assert.Equal(t, "/hailuo/v2/query/video_generation/nexhina-idc-task", r.URL.Path)
			_, _ = io.WriteString(w, fx.queryBodyRes.Load().(string))
		default:
			t.Errorf("unexpected provider method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(provider.Close)

	channel := &model.Channel{
		Type: constant.ChannelTypeMiniMaxLink, Status: common.ChannelStatusEnabled,
		Name: "Nexhina IDC", Key: "provider-fixture-key", BaseURL: &provider.URL,
		Models: "customer-video-idc", Group: "default",
		ModelMapping: common.GetPointer(`{"customer-video-idc":"` + providerModel + `","` + providerModel + `":"another-route","MiniMax-H3-idc":"different-route"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: constant.VideoUpstreamProtocolNexhinaHailuoV2})
	body, err := common.Marshal(AddChannelRequest{Mode: "single", Channel: channel})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(body))
	AddChannel(ctx)
	require.JSONEq(t, `{"success":true,"message":""}`, recorder.Body.String())

	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 2)
	assert.Equal(t, "customer-video-idc", catalog[1].API.Video.Creation.Model)
	assert.Equal(t, 1, catalog[1].API.Video.Creation.ContentTypes[1].MaxItems)

	rejected := fx.submitCreateBody(`{"model":"customer-video-idc","content":[{"type":"text","text":"A landscape"}],"duration":16,"resolution":"768p","ratio":"16:9"}`)
	assert.Contains(t, rejected, "invalid_request")
	assert.Zero(t, fx.createCalls.Load())
	assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())

	const validRequest = `{"model":"customer-video-idc","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"768p","ratio":"16:9"}`
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(validRequest))
	task := fx.loadTask(id)
	fx.channelID = task.ChannelId
	assert.Equal(t, providerModel, task.Properties.UpstreamModelName)
	assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
	remaining, used := fx.tokenQuota()
	assert.Zero(t, remaining)
	assert.Equal(t, hold, used)

	const tasksPath = "/api/v3/contents/generations/tasks"
	path := tasksPath + "/" + id
	w := minimaxGet(t, fx, path)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), id)
	denied := httptest.NewRecorder()
	repeat := httptest.NewRequest(http.MethodPost, tasksPath, strings.NewReader(validRequest))
	repeat.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
	repeat.Header.Set("Content-Type", "application/json")
	fx.engine.ServeHTTP(denied, repeat)
	assert.Equal(t, http.StatusUnauthorized, denied.Code)
	assert.EqualValues(t, 1, fx.createCalls.Load())
	// The write check may mark the token exhausted, but the same owner can
	// still read the accepted task while its hold is outstanding.
	// Place it inside the list's exclusive upper timestamp bound.
	require.NoError(t, fx.db.Model(&model.Task{}).Where("id = ?", task.ID).UpdateColumn("created_at", common.GetTimestamp()-1).Error)
	w = minimaxGet(t, fx, tasksPath)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), id)

	fx.pollOnce(id)
	assert.EqualValues(t, model.TaskStatusQueued, fx.loadTask(id).Status)
	const success = `{"task":{"id":"nexhina-idc-task","model":"MiniMax-H3","status":"succeeded","task_type":"generation","modality":"video","content":{"url":"https://media.example.test/video.mp4"},"resolution":"768P","ratio":"16:9","duration":4,"usage":{"output_seconds":4}}}`
	fx.queryBodyRes.Store(strings.Replace(success, `"output_seconds":4`, `"output_seconds":null`, 1))
	fx.pollOnce(id)
	task = fx.loadTask(id)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	require.Equal(t, model.TaskBillingStateAwaitingUsage, task.BillingState)
	assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())

	fx.queryBodyRes.Store(success)
	service.ReconcileTaskUsage(context.Background())
	task = fx.loadTask(id)
	require.Equal(t, model.TaskBillingStateSettled, task.BillingState)
	assert.Equal(t, providerModel, task.Properties.UpstreamModelName)
	assert.Equal(t, map[string]float64{"output_seconds": 4}, task.PrivateData.AsyncBilling.OfficialUsage)
	w = minimaxGet(t, fx, path)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"succeeded"`)
	assert.Contains(t, w.Body.String(), `"model":"customer-video-idc"`)
	assert.NotContains(t, w.Body.String(), "MiniMax-H3")

	// Re-reading the same terminal observation cannot charge or refund twice.
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	assert.Equal(t, seedanceFundsInitialQuota-final, fx.userQuota())
	remaining, used = fx.tokenQuota()
	assert.Equal(t, hold-final, remaining)
	assert.Equal(t, final, used)
	logs := fx.logsByType()
	require.Len(t, logs[model.LogTypeRefund], 1)
	assert.Equal(t, hold-final, logs[model.LogTypeRefund][0].Quota)
	assert.EqualValues(t, 1, fx.createCalls.Load())

	otherKey := strings.Repeat("e", 32)
	require.NoError(t, fx.db.Create(&model.Token{UserId: fx.userID, Key: otherKey, Status: common.TokenStatusEnabled, ExpiredTime: -1, Group: "default"}).Error)
	other := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer sk-"+otherKey)
	fx.engine.ServeHTTP(other, request)
	assert.Equal(t, http.StatusNotFound, other.Code)

	token := fx.db.Model(&model.Token{}).Where("id = ?", fx.userID)
	require.NoError(t, token.Update("allow_ips", "127.0.0.1").Error)
	assert.Equal(t, http.StatusForbidden, minimaxGet(t, fx, path).Code)
	require.NoError(t, token.Updates(map[string]any{"allow_ips": "", "expired_time": common.GetTimestamp() - 1}).Error)
	assert.Equal(t, http.StatusUnauthorized, minimaxGet(t, fx, path).Code)
	require.NoError(t, token.Updates(map[string]any{"expired_time": -1, "status": common.TokenStatusDisabled}).Error)
	assert.Equal(t, http.StatusUnauthorized, minimaxGet(t, fx, path).Code)
}

func TestMiniMaxNexhinaCreateAuthRejectionReleasesOnlyVerifiedHold(t *testing.T) {
	const hold = 600_000 // Frozen 15-second maximum at $0.08 and 500,000 quota/USD.
	const refusal = `{"error":{"code":"","type":"nexhina_error","message":"Invalid provider credential"}}`
	for _, tc := range []struct {
		name       string
		body       string
		status     model.TaskCreateAttemptStatus
		holdState  model.TaskCreateAttemptBillingHoldState
		quotaSpent int
	}{
		{"verified refusal", refusal, model.TaskCreateAttemptRejected, model.TaskCreateAttemptBillingReleased, 0},
		{"task ID contradicts refusal", `{"task_id":"maybe-accepted",` + refusal[1:], model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
		{"unverified 401", `<html>Unauthorized</html>`, model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
		{"incomplete response within read limit", refusal + strings.Repeat(" ", 32<<10) + "trailing content", model.TaskCreateAttemptUnknown, model.TaskCreateAttemptBillingHeld, hold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr": `{"customer-video":"tier(\"nexhina\", u(\"output_seconds\") * 0.08)"}`,
			}))
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/hailuo/v2/video_generation", r.URL.Path)
				fx.createCalls.Add(1)
				var attempt model.TaskCreateAttempt
				if assert.NoError(t, fx.db.First(&attempt).Error) {
					assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
					assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
					assert.Equal(t, hold, attempt.HeldQuota)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(provider.Close)
			require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Updates(map[string]any{
				"base_url": provider.URL,
				"settings": `{"video_upstream_protocol":"nexhina_hailuo_v2","asset_upstream_protocol":"none"}`,
			}).Error)
			response := fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`)
			assert.NotContains(t, response, "maybe-accepted")
			var attempt model.TaskCreateAttempt
			require.NoError(t, fx.db.First(&attempt).Error)
			assert.Equal(t, tc.status, attempt.Status)
			assert.Equal(t, tc.holdState, attempt.BillingHoldState)
			assert.Equal(t, seedanceFundsInitialQuota-tc.quotaSpent, fx.userQuota())
			remaining, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-tc.quotaSpent, remaining)
			assert.Equal(t, tc.quotaSpent, used)
			var tasks int64
			require.NoError(t, fx.db.Model(&model.Task{}).Count(&tasks).Error)
			assert.Zero(t, tasks)
			assert.EqualValues(t, 1, fx.createCalls.Load())
		})
	}
}
