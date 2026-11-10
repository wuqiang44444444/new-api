package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
	return newMiniMaxFundsFixtureWithQuery(t, nil, expectedHold...)
}

func newMiniMaxFundsFixtureWithQuery(t *testing.T, query func(http.ResponseWriter), expectedHold ...int) *seedanceFundsFixture {
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
		if query != nil {
			query(w)
			return
		}
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
		Source: model.LongText(source), SourceHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(source))), Active: true, Enabled: true}
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
	// Hosted-delivery registration facts join the fixture schema.
	require.NoError(t, fx.db.AutoMigrate(&model.Option{}, &model.TaskVideoResultObject{}))
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
	assert.Equal(t, "/docs/api-reference/videos/h3", api.DocumentationPath)
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
	fx.pollOnce(id)
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

	// Hosted delivery: the client path never queries the provider or fetches
	// media. Until the transfer worker stores the object, delivery is preparing.
	before := fx.queryCalls.Load()
	w := minimaxGet(t, fx, "/v1/videos/"+id+"/content")
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "video_delivery_preparing")
	assert.Equal(t, before, fx.queryCalls.Load(), "hosted delivery must not query the provider on the client path")
	assert.EqualValues(t, 1, fx.createCalls.Load())

	fx.queryBodyRes.Store(strings.Replace(minimaxSuccess, `"video_output":7`, `"video_output":99`, 1))
	task = fx.loadTask(id)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
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
		fx.queryStatus.Store(int32(observation.status))
		fx.queryBodyRes.Store(observation.body)
		task = fx.loadTask(id)
		require.Error(t, service.RefreshVideoTask(context.Background(), &task))
		before = fx.queryCalls.Load()
		w = minimaxGet(t, fx, "/api/v3/contents/generations/tasks/"+id)
		require.Equal(t, 200, w.Code)
		assert.Equal(t, before, fx.queryCalls.Load())
		var public dto.ModelArkVideoTask
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &public))
		assert.Equal(t, "succeeded", public.Status)
		require.NotNil(t, public.Content)
		assert.Equal(t, "preparing", public.Content.DeliveryStatus)
		assert.Empty(t, public.Content.VideoURL, "upstream links and unsigned placeholders must not surface")
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

func TestMiniMaxStatusComesFromBackgroundPolling(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`))
	fx.queryBodyRes.Store(`{"task_id":"jd-task-1","task_status":"running"}`)
	path := "/api/v3/contents/generations/tasks/" + id
	w := minimaxGet(t, fx, path)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"queued"`)
	assert.Zero(t, fx.queryCalls.Load())

	fx.pollOnce(id)
	w = minimaxGet(t, fx, path)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"running"`)
	assert.EqualValues(t, 1, fx.queryCalls.Load())
	assert.Zero(t, fx.loadTask(id).PrivateData.VideoUpstreamNextQueryAt)
}

func TestMiniMaxTemporaryQueryFailurePreservesTaskAndHold(t *testing.T) {
	oldCutoff := constant.TaskPollMaxFailures
	constant.TaskPollMaxFailures = 1
	t.Cleanup(func() { constant.TaskPollMaxFailures = oldCutoff })

	for _, tc := range []struct {
		name       string
		status     int
		retryAfter string
		delay      int64
		httpDate   bool
	}{
		{name: "transport error"},
		{name: "upstream unavailable", status: http.StatusServiceUnavailable},
		{name: "invalid retry after", status: http.StatusTooManyRequests, retryAfter: "-90"},
		{name: "retry after seconds", status: http.StatusTooManyRequests, retryAfter: "90", delay: 90},
		{name: "retry after date", status: http.StatusTooManyRequests, httpDate: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queryFailure atomic.Bool
			retryDate := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
			fx := newMiniMaxFundsFixtureWithQuery(t, func(w http.ResponseWriter) {
				if !queryFailure.Load() {
					_, _ = io.WriteString(w, `{"task_id":"jd-task-1","task_status":"pending"}`)
					return
				}
				if tc.status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if assert.NoError(t, err) {
						_ = conn.Close()
					}
					return
				}
				retryAfter := tc.retryAfter
				if tc.httpDate {
					retryAfter = retryDate.Format(http.TimeFormat)
				}
				w.Header().Set("Retry-After", retryAfter)
				w.WriteHeader(tc.status)
			})
			id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}]}`))
			path := "/api/v3/contents/generations/tasks/" + id
			fx.pollOnce(id)
			last := fx.loadTask(id)
			require.EqualValues(t, model.TaskStatusQueued, last.Status)
			queryFailure.Store(true)

			before := time.Now().Unix()
			fx.pollOnce(id)
			calls := fx.queryCalls.Load()
			w := minimaxGet(t, fx, path)
			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, calls, fx.queryCalls.Load(), "client GET uses only the saved observation")
			var public dto.ModelArkVideoTask
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &public))
			assert.Equal(t, "queued", public.Status)
			stored := fx.loadTask(id)
			assert.Equal(t, last.Status, stored.Status)
			assert.Equal(t, last.Progress, stored.Progress)
			assert.Equal(t, last.Data, stored.Data)
			assert.Equal(t, last.FailReason, stored.FailReason)
			assert.Equal(t, last.UpdatedAt, stored.UpdatedAt)
			assert.Zero(t, stored.PrivateData.PollFailures)
			assert.Equal(t, model.TaskBillingStatePending, stored.BillingState)
			assert.Equal(t, last.Quota, stored.Quota)
			assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, fx.userQuota())
			remaining, used := fx.tokenQuota()
			assert.Equal(t, seedanceFundsInitialQuota-minimaxFundsHold, remaining)
			assert.Equal(t, minimaxFundsHold, used)

			nextAt := stored.PrivateData.VideoUpstreamNextQueryAt
			if tc.httpDate {
				assert.Equal(t, retryDate.Unix(), nextAt)
			} else if tc.delay > 0 {
				assert.GreaterOrEqual(t, nextAt, before+tc.delay)
				assert.LessOrEqual(t, nextAt, time.Now().Unix()+tc.delay+1)
			} else {
				assert.Zero(t, nextAt)
			}
			queryFailure.Store(false)
			fx.pollOnce(id)
			if nextAt > 0 {
				assert.Equal(t, calls, fx.queryCalls.Load(), "background polling respects Retry-After")
			} else {
				assert.Equal(t, calls+1, fx.queryCalls.Load(), "the next background pass can query without a Retry-After deadline")
			}

			if tc.retryAfter == "90" {
				// An observation from a query started on the old snapshot must
				// not replace the later deadline recorded by the 429.
				require.NoError(t, service.RefreshVideoTask(context.Background(), &last))
				assert.Equal(t, nextAt, fx.loadTask(id).PrivateData.VideoUpstreamNextQueryAt)
				calls = fx.queryCalls.Load()
				stored = fx.loadTask(id)
				stored.PrivateData.VideoUpstreamNextQueryAt = before - 1
				require.NoError(t, fx.db.Model(&stored).UpdateColumn("private_data", stored.PrivateData).Error)
				fx.pollOnce(id)
				assert.Equal(t, calls+1, fx.queryCalls.Load(), "background polling resumes after the deadline")
			}
		})
	}
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
