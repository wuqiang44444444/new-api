package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	"github.com/QuantumNous/new-api/plugins"
	taskminimax "github.com/QuantumNous/new-api/relay/channel/task/minimax"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const minimaxFundsHold = 750_000 // 6 seconds * $0.25 * 500000 quota/USD.
const minimaxSuccess = `{"task_id":"jd-task-1","task_status":"success","content":[{"video_url":{"url":"https://8.8.8.8/video.mp4"}}],"usage":{"video_output":7}}`

// Reuse the established SQLite/auth/funding fixture, replacing only the typed
// channel, artifact and provider. Requests still traverse the production chain.
func newMiniMaxFundsFixture(t *testing.T, expectedHold ...int) *seedanceFundsFixture {
	t.Helper()
	heldQuota := minimaxFundsHold
	if len(expectedHold) > 0 {
		heldQuota = expectedHold[0]
	}
	fx := newSeedanceFundsFixture(t)
	require.NoError(t, fx.db.AutoMigrate(&model.ErrorEvent{}))
	fx.createBodyRes.Store(`{"result":{"task_id":"jd-task-1","status":"pending"}}`)
	fx.queryBodyRes.Store(`{"task_id":"jd-task-1","task_status":"pending"}`)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer provider-fixture-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			assert.Equal(t, "/v1/task/submit", r.URL.Path)
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
				assert.Equal(t, heldQuota, attempt.HeldQuota)
				var frozen struct {
					PluginKey string `json:"plugin_key"`
				}
				assert.NoError(t, common.Unmarshal(attempt.FrozenConnectionSnapshot, &frozen))
				assert.Equal(t, jsplugin.MinimaxPluginKey, frozen.PluginKey)
			}
			w.WriteHeader(int(fx.createStatus.Load()))
			_, _ = io.WriteString(w, fx.createBodyRes.Load().(string))
			return
		}
		assert.Equal(t, "/v1/task/jd-task-1", r.URL.Path)
		fx.queryCalls.Add(1)
		w.WriteHeader(int(fx.queryStatus.Load()))
		_, _ = io.WriteString(w, fx.queryBodyRes.Load().(string))
	}))
	t.Cleanup(provider.Close)
	var channel model.Channel
	require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
	channel.Type = constant.ChannelTypeMiniMaxLink
	channel.BaseURL = &provider.URL
	channel.ModelMapping = common.GetPointer(`{"customer-video":"MiniMax-H3"}`)
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: constant.VideoUpstreamProtocolJdCloudTaskV1})
	require.NoError(t, fx.db.Save(&channel).Error)
	source := plugins.MinimaxSource()
	row := model.TaskPlugin{Key: jsplugin.MinimaxPluginKey, Version: plugins.MinimaxVersion(), APIVersion: 3,
		Source: source, SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Active: true, Enabled: true}
	require.NoError(t, fx.db.Create(&row).Error)
	previousStore := minimaxplugin.Default
	minimaxplugin.Default = minimaxplugin.NewStore()
	t.Cleanup(func() { minimaxplugin.Default = previousStore })
	require.NoError(t, minimaxplugin.Default.SyncSnapshot(context.Background(), []model.TaskPlugin{row}))
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		if platform == taskminimax.Platform {
			return &taskminimax.TaskAdaptor{}
		}
		return nil
	}
	fx.engine = gin.New()
	fx.engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(),
		middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(),
		middleware.ModelArkVideoCreateConvert(), middleware.ResolveStandardVideoChannel(), RelayTask)
	fx.engine.GET("/api/v3/contents/generations/tasks/:task_id", middleware.TokenAuth(), ModelArkVideoGet)
	fx.engine.GET("/v1/videos/:task_id/content", middleware.TokenAuth(), func(c *gin.Context) {
		if !proxyLinkVideoContent(c) {
			c.Status(http.StatusNotFound)
		}
	})
	return fx
}

func minimaxDueTask(t *testing.T, fx *seedanceFundsFixture, id string) *model.Task {
	t.Helper()
	task := fx.loadTask(id)
	task.PrivateData.VideoUpstreamNextQueryAt = 0
	_, err := task.UpdateWithStatus(task.Status)
	require.NoError(t, err)
	return &task
}

func minimaxGet(t *testing.T, fx *seedanceFundsFixture, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
	fx.engine.ServeHTTP(w, r)
	return w
}

func TestMiniMaxStandardEntryFundingAndContent(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	// Explicitly authorize and configure Auto; authentication must stay real.
	oldUsable, oldAuto := setting.UserUsableGroups2JSONString(), setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","auto":"Auto"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	// The standard entry must resolve auto groups before authorization/selection.
	require.NoError(t, fx.db.Model(&model.Token{}).Where("id = ?", fx.userID).Update("group", "auto").Error)
	catalog, err := model.GetConfiguredMiniMaxPublicModels()
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	api := catalog[0].API.Video
	request := map[string]any{"model": "customer-video", "content": []any{map[string]any{"type": "text", "text": "A landscape"}}}
	for _, p := range api.Creation.Parameters {
		if p.DefaultValue != nil {
			request[p.Name] = p.DefaultValue
		}
	}
	assert.Equal(t, 6, request["duration"])
	assert.Equal(t, "768p", request["resolution"])
	assert.Equal(t, "16:9", request["ratio"])
	require.Len(t, api.Creation.ContentTypes, 4)
	assert.Equal(t, "/docs/api-reference/videos/minimax", api.DocumentationPath)
	assert.Equal(t, []string{"type", "text"}, api.Creation.ContentTypes[0].RequiredFields)
	assert.Equal(t, "text", api.Creation.ContentTypes[0].Type)
	body, err := common.Marshal(request)
	require.NoError(t, err)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(string(body)))
	task := fx.loadTask(id)
	require.Equal(t, taskminimax.Platform, task.Platform)
	require.True(t, task.HasMiniMaxBillingFacts())
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"resolution":"768p"`)
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"ratio":"16:9"`)
	require.True(t, task.HasTaskUsageBilling())
	assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
	assert.Equal(t, "second", task.PrivateData.AsyncBilling.TieredSnapshot.UsageUnits["duration_seconds"])
	assert.NotContains(t, task.PrivateData.AsyncBilling.TieredSnapshot.UsageUnits, "tokens")
	assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
	assert.JSONEq(t, `{"model":"MiniMax-H3","content":[{"type":"text","text":"A landscape"}],"parameters":{"duration":6,"resolution":"768P","ratio":"16:9","prompt_optimizer":true,"watermark":false}}`, string(fx.createRequestBody()))
	var attempt model.TaskCreateAttempt
	require.NoError(t, fx.db.First(&attempt).Error)
	assert.Equal(t, model.TaskCreateAttemptComplete, attempt.Status)
	assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempt.BillingHoldState)
	// Changing live channel data cannot affect a frozen task.
	require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Updates(map[string]any{"key": "rotated", "base_url": "http://127.0.0.1:1"}).Error)
	staleBilling := fx.loadTask(id)
	stalePoll := fx.loadTask(id)
	fx.queryBodyRes.Store(minimaxSuccess)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	settled := fx.loadTask(id)
	require.EqualValues(t, model.TaskStatusSuccess, settled.Status)
	require.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
	assert.Equal(t, map[string]int{"video_output": 7}, settled.PrivateData.AsyncBilling.ActualUsageEvidence)
	assert.Equal(t, taskminimax.CreditUsageSource, settled.PrivateData.AsyncBilling.ActualUsageSource)
	assert.False(t, settled.PrivateData.AsyncBilling.ActualUsageReported)
	assert.Zero(t, settled.PrivateData.AsyncBilling.ActualTokens)
	assert.NotContains(t, string(settled.Data), "video_output")
	assert.NotContains(t, string(settled.Data), "8.8.8.8")
	assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
	assert.Zero(t, service.ReconcileTaskBilling(context.Background(), 10).Scanned)
	var deliveries []model.TaskBillingDelivery
	require.NoError(t, fx.db.Where("task_row_id = ?", settled.ID).Find(&deliveries).Error)
	require.Len(t, deliveries, 2, "create and settlement use the durable log outbox")
	// Stale observation and billing writers must not reopen a settled task.
	stalePoll.Status = model.TaskStatusSuccess
	_, err = stalePoll.UpdateWithStatus(model.TaskStatusSuccess)
	require.NoError(t, err)
	require.NoError(t, staleBilling.UpdateBilling())
	assert.Equal(t, model.TaskBillingStateSettled, fx.loadTask(id).BillingState)

	// Cold content lookup ignores the next scheduled background query. The
	// protected media client is stubbed: no external network or credential leak.
	mediaClient := service.GetSSRFProtectedHTTPClient()
	oldTransport := mediaClient.Transport
	t.Cleanup(func() { mediaClient.Transport = oldTransport })
	mediaClient.Transport = fetchModelsRoundTripper(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://8.8.8.8/video.mp4", r.URL.String())
		assert.Empty(t, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"video/mp4"}}, Body: io.NopCloser(strings.NewReader("video-bytes"))}, nil
	})
	before := fx.queryCalls.Load()
	w := minimaxGet(t, fx, "/v1/videos/"+id+"/content")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "video-bytes", w.Body.String())
	assert.Equal(t, before+1, fx.queryCalls.Load())
	assert.EqualValues(t, 1, fx.createCalls.Load())

	fx.queryBodyRes.Store(strings.Replace(minimaxSuccess, `"video_output":7`, `"video_output":99`, 1))
	require.NoError(t, service.RefreshVideoTask(context.Background(), minimaxDueTask(t, fx, id)))
	conflicting := fx.loadTask(id)
	assert.Equal(t, map[string]int{"video_output": 7}, conflicting.PrivateData.AsyncBilling.ActualUsageEvidence)
	require.NotNil(t, conflicting.PrivateData.AsyncBilling.UsageDiscrepancy)
	assert.Equal(t, model.TaskBillingStateSettled, conflicting.BillingState)

	for _, observation := range []struct {
		status int
		body   string
	}{
		{404, `{}`}, {200, `{"task_id":"foreign","task_status":"success"}`},
		{200, `{"task_id":"jd-task-1","task_status":"running"}`},
	} {
		minimaxDueTask(t, fx, id)
		fx.queryStatus.Store(int32(observation.status))
		fx.queryBodyRes.Store(observation.body)
		w = minimaxGet(t, fx, "/api/v3/contents/generations/tasks/"+id)
		require.Equal(t, 200, w.Code)
		var public dto.ModelArkVideoTask
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &public))
		assert.Equal(t, "succeeded", public.Status)
		require.NotNil(t, public.Content)
		assert.Contains(t, public.Content.VideoURL, id+"/content")
		assert.EqualValues(t, model.TaskStatusSuccess, fx.loadTask(id).Status)
		assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
	}
}

func TestMiniMaxCancelledAndFailedFundingRecovery(t *testing.T) {
	for _, status := range []string{"cancelled", "failed"} {
		t.Run(status, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`))
			body := `{"task_id":"jd-task-1","task_status":"cancelled"}`
			if status == "failed" {
				body = `{"task_id":"jd-task-1","task_status":"failed","error":{"code":400,"message":"content rejected"}}`
			}
			fx.queryBodyRes.Store(body)
			// A real refund transaction fault must leave durable retry state.
			const callback = "minimax_test_refund_failure"
			require.NoError(t, fx.db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "users" {
					tx.AddError(errors.New("injected refund storage failure"))
				}
			}))
			t.Cleanup(func() { fx.db.Callback().Update().Remove(callback) })
			task := fx.loadTask(id)
			require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
			failed := fx.loadTask(id)
			require.Equal(t, model.TaskBillingStateFailed, failed.BillingState)
			assert.True(t, failed.Status.ShouldRefundOnTerminal())
			require.NotNil(t, failed.PrivateData.AsyncBilling.TargetQuota)
			assert.Zero(t, *failed.PrivateData.AsyncBilling.TargetQuota)
			assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
			require.NoError(t, fx.db.Callback().Update().Remove(callback))
			failed.PrivateData.AsyncBilling.NextRetryAt = 0
			require.NoError(t, failed.UpdateBilling())
			assert.Equal(t, 1, service.ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			remaining, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota, remaining)
			assert.Zero(t, used)
			assert.Equal(t, model.TaskBillingStateSettled, fx.loadTask(id).BillingState)
			assert.Zero(t, service.ReconcileTaskBilling(context.Background(), 10).Scanned)
			assert.EqualValues(t, 1, fx.createCalls.Load())
		})
	}
}

func TestMiniMaxPollingSchedulePersistsUnchangedObservation(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`))
	fx.queryBodyRes.Store(`{"task_id":"jd-task-1","task_status":"running"}`)
	task := fx.loadTask(id)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	task = *minimaxDueTask(t, fx, id)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	stored := fx.loadTask(id)
	assert.Greater(t, stored.PrivateData.VideoUpstreamNextQueryAt, time.Now().Unix())
	count := fx.queryCalls.Load()
	require.NoError(t, service.RefreshVideoTask(context.Background(), &stored))
	assert.Equal(t, count, fx.queryCalls.Load())
}

func TestMiniMaxCatalogFailuresAreExplicit(t *testing.T) {
	for _, defect := range []string{"missing_artifact", "unknown_protocol", "missing_model", "database_error"} {
		t.Run(defect, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			switch defect {
			case "missing_artifact":
				require.NoError(t, fx.db.Where("key = ?", jsplugin.MinimaxPluginKey).Delete(&model.TaskPlugin{}).Error)
			case "unknown_protocol":
				require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Update("settings", `{"video_upstream_protocol":"unknown"}`).Error)
			case "missing_model":
				require.NoError(t, fx.db.Model(&model.Channel{}).Where("id = ?", fx.channelID).Update("model_mapping", `{"customer-video":"undeclared"}`).Error)
			case "database_error":
				require.NoError(t, fx.db.Migrator().DropTable(&model.TaskPlugin{}))
			}
			catalog, err := standardVideoPublicCatalog()
			require.Error(t, err)
			assert.Nil(t, catalog)
		})
	}
}

func TestMiniMaxCreateRejectionAndUnknownBoundaries(t *testing.T) {
	t.Run("unsupported adaptive is rejected before attempt and hold", func(t *testing.T) {
		fx := newMiniMaxFundsFixture(t)
		response := fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"ratio":"adaptive"}`)
		assert.Contains(t, response, "invalid_request")
		var count int64
		require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&count).Error)
		assert.Zero(t, count)
		assert.Zero(t, fx.createCalls.Load())
		assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
	})
	t.Run("unregistered create acceptance stays unknown and never resends", func(t *testing.T) {
		fx := newMiniMaxFundsFixture(t)
		fx.createBodyRes.Store(`{"result":{"task_id":"jd-task-1","status":"running"}}`)
		fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`)
		var attempt model.TaskCreateAttempt
		require.NoError(t, fx.db.First(&attempt).Error)
		assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
		assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
		var count int64
		require.NoError(t, fx.db.Model(&model.Task{}).Count(&count).Error)
		assert.Zero(t, count)
		assert.EqualValues(t, 1, fx.createCalls.Load())
		assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
	})
}
