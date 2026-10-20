package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Free synthetic requests keep this routing fixture independent of unrelated
// long-lived workers in the shared process pool. Billing has its own fixtures.
func newNativeRelayErrorFixture(t *testing.T) *nativeDiagFixture {
	t.Helper()
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldBatch, oldCount := common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken
	oldPrice, oldGroup := ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
	oldEvidence := system_setting.GetTaskRequestEvidenceConfig()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken = oldRedis, oldBatch, oldCount
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		system_setting.SetTaskRequestEvidenceConfig(oldEvidence)
		require.NoError(t, sqlDB.Close())
	})
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.BatchUpdateEnabled, constant.CountToken = false, false, false
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Log{}, &model.ErrorEvent{}, &model.QuotaData{}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-2":0}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	withTieredBillingConfig(t, map[string]string{}, map[string]string{})
	service.InitHttpClient()
	user := model.User{Id: 8961, Username: "native-relay-error", Quota: 100000, Group: "default", Status: common.UserStatusEnabled}
	token := model.Token{Id: 8961, UserId: user.Id, Key: strings.Repeat("d", 32), RemainQuota: 100000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&token).Error)
	var calls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("apim-request-id", "attempt-one")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":"RateLimitReached","message":"synthetic limit"}}`)
	}))
	t.Cleanup(provider.Close)
	require.NoError(t, db.Create(&model.Channel{Id: user.Id, Type: constant.ChannelTypeAzure, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled}).Error)
	return &nativeDiagFixture{db: db, user: user, token: token, calls: &calls}
}

func TestAzureImageRetryFinalErrorEvent(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		secondStatus, clientStatus int
		mapping, wantReason        string
		transport, noCandidates    bool
	}{
		{name: "two rate limited channels", secondStatus: 429, clientStatus: 429, wantReason: "upstream_rate_limited"},
		{name: "mapped rate limit", secondStatus: 429, clientStatus: 503, mapping: `{"429":503}`, wantReason: "upstream_rate_limited"},
		{name: "retry succeeds", secondStatus: 200, clientStatus: 200},
		{name: "retry connection failure", clientStatus: 500, transport: true, wantReason: "upstream_transport_error"},
		{name: "retry selection fails", clientStatus: 500, noCandidates: true, wantReason: "channel_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newNativeRelayErrorFixture(t)
			oldCache, oldRetry, oldBan, oldErrorLog := common.MemoryCacheEnabled, common.RetryTimes, common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled
			oldRanges := operation_setting.AutomaticRetryStatusCodeRanges
			common.MemoryCacheEnabled, common.RetryTimes, common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = false, 1, false, true
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.RetryTimes, common.AutomaticDisableChannelEnabled, constant.ErrorLogEnabled = oldCache, oldRetry, oldBan, oldErrorLog
				operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
			})
			var first model.Channel
			require.NoError(t, fixture.db.First(&first, fixture.user.Id).Error)
			first.Id, first.Type, first.Models, first.Group = 149, constant.ChannelTypeAzure, "gpt-image-2", "default"
			first.Priority = common.GetPointer(int64(1))
			if tc.noCandidates {
				provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fixture.calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					assert.NoError(t, fixture.db.Model(&model.Ability{}).Where("model = ?", "gpt-image-2").Update("enabled", false).Error)
					w.Header().Set("apim-request-id", "attempt-one")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = io.WriteString(w, `{"error":{"code":"RateLimitReached","message":"synthetic limit"}}`)
				}))
				t.Cleanup(provider.Close)
				first.BaseURL = &provider.URL
			}
			require.NoError(t, fixture.db.Create(&first).Error)
			var secondCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				secondCalls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				if tc.transport {
					conn, _, err := w.(http.Hijacker).Hijack()
					if assert.NoError(t, err) {
						_ = conn.Close()
					}
					return
				}
				w.Header().Set("apim-request-id", "attempt-two")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.secondStatus)
				if tc.secondStatus == 200 {
					_, _ = io.WriteString(w, `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":12,"output_tokens":20,"total_tokens":32}}`)
					return
				}
				_, _ = io.WriteString(w, `{"error":{"code":"RateLimitReached","message":"synthetic second limit"}}`)
			}))
			t.Cleanup(provider.Close)
			second := first
			second.Id, second.BaseURL, second.Priority = 155, &provider.URL, common.GetPointer(int64(0))
			second.StatusCodeMapping = &tc.mapping
			require.NoError(t, fixture.db.Create(&second).Error)
			for _, channel := range []model.Channel{first, second} {
				require.NoError(t, fixture.db.Create(&model.Ability{Group: "default", Model: "gpt-image-2", ChannelId: channel.Id, Enabled: true, Priority: channel.Priority}).Error)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			sink := clienterrlog.NewSink(ctx, 8, func(clienterrlog.Event) error { return nil })
			engine := gin.New()
			engine.Use(sink.Middleware())
			engine.POST("/v1/images/edits", func(c *gin.Context) {
				c.Set("route_tag", "relay")
				c.Set(common.RequestIdKey, "native-retry-request")
				common.SetContextKey(c, constant.ContextKeyUserId, fixture.user.Id)
				common.SetContextKey(c, constant.ContextKeyUserQuota, fixture.user.Quota)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				common.SetContextKey(c, constant.ContextKeyTokenId, fixture.token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, fixture.token.Key)
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &first, "gpt-image-2"))
			}, func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
			response := submitNativeImageCreate(t, engine, "/v1/images/edits", `{"model":"gpt-image-2","prompt":"synthetic drawing","images":[{"image_url":"https://example.com/image.png"}]}`, false)
			require.Equal(t, tc.clientStatus, response.Code, response.Body.String())
			assert.EqualValues(t, 1, fixture.calls.Load())
			if tc.noCandidates {
				assert.Zero(t, secondCalls.Load())
			} else {
				assert.EqualValues(t, 1, secondCalls.Load())
			}
			var logs []model.Log
			require.NoError(t, fixture.db.Where("request_id = ? AND type = ?", "native-retry-request", model.LogTypeError).Order("id ASC").Find(&logs).Error)
			if tc.clientStatus == 200 {
				assert.Zero(t, sink.Health().Accepted)
				require.Len(t, logs, 1)
				return
			}
			require.Eventually(t, func() bool { return sink.Health().Persisted == 1 || sink.Health().PersistFailed > 0 }, 3*time.Second, time.Millisecond)
			require.Zero(t, sink.Health().PersistFailed)
			var events []model.ErrorEvent
			require.NoError(t, fixture.db.Find(&events).Error)
			require.Len(t, events, 1)
			event := events[0]
			assert.Equal(t, "gpt-image-2", event.ModelName)
			assert.Equal(t, tc.wantReason, event.Reason)
			assert.Equal(t, tc.clientStatus, event.Status)
			if tc.noCandidates {
				assert.Zero(t, event.ChannelId)
				assert.Equal(t, "channel_selection", event.Stage)
				assert.Empty(t, event.UpstreamRequestId)
				assert.NotContains(t, event.Detail, "synthetic limit")
				require.Len(t, logs, 1)
				assert.Equal(t, 149, logs[0].ChannelId)
				return
			}
			assert.Equal(t, 155, event.ChannelId)
			require.Len(t, logs, 2)
			assert.Equal(t, 149, logs[0].ChannelId)
			assert.Equal(t, 155, logs[1].ChannelId)
			assert.Equal(t, logs[1].UpstreamRequestId, event.UpstreamRequestId)
			if tc.transport {
				assert.Empty(t, event.UpstreamRequestId)
				assert.NotContains(t, event.Detail, "synthetic limit")
			} else {
				assert.Equal(t, "attempt-two", event.UpstreamRequestId)
				assert.Contains(t, event.Detail, "synthetic second limit")
				assert.NotContains(t, event.Detail, "synthetic limit")
			}
		})
	}
}
