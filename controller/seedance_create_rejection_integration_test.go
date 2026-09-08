package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceCreateRejectionReleasesHoldWithoutRetry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol kitdto.VideoUpstreamProtocol
		status   int
		body     string
		rejected bool
	}{
		{"TokenSave group permission", kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"error":{"code":"group_model_permission_denied","message":"Model unavailable in current group"}}`, true},
		{"official inactive key", kitdto.VideoUpstreamProtocolModelArkV3Volcengine, 401, `{"error":{"code":"AuthenticationError","message":"The API key status is not active."}}`, true},
		{"CMCC invalid key", kitdto.VideoUpstreamProtocolModelArkV3CMCC, 403, `{"message":"api key is invalid"}`, true},
		{"conflicting task id retains hold", kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, 403, `{"data":{"task_id":"provider-task-1"},"error":{"code":"group_model_permission_denied","message":"Model unavailable in current group"}}`, false},
		{"unverified server error retains hold", kitdto.VideoUpstreamProtocolModelArkV3Volcengine, 503, `{"error":{"code":"AuthenticationError","message":"Unavailable"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service.InitHttpClient()
			previousDBType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			t.Cleanup(func() { common.SetDatabaseTypes(previousDBType, previousLogType) })
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.TaskCreateAttempt{}, &model.TaskCreateIdempotency{}, &model.UserSubscription{}))
			oldLog, oldCache, oldRedis, oldBatch, oldConsume := model.LOG_DB, common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled
			t.Setenv("LOG_SQL_DSN", "")
			require.NoError(t, model.InitLogDB())
			model.LOG_DB = db
			common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = false, false, false, false
			t.Cleanup(func() {
				model.LOG_DB = oldLog
				common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled = oldCache, oldRedis, oldBatch, oldConsume
			})
			saved := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(k, v string) error { saved[k] = v; return nil }))
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_mode":           `{"customer-video":"tiered_expr"}`,
				"billing_setting.billing_expr":           `{"customer-video":"tier(\"fixed\", 1400)"}`,
				"task_billing_setting.preconsume_tokens": `{"customer-video":100000}`,
			}))
			user := model.User{Id: 8191, Username: "rejection-fixture", Quota: 10000, Status: common.UserStatusEnabled, Group: "default"}
			user.SetSetting(kitdto.UserSetting{BillingPreference: "wallet_only"})
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{Id: 8191, UserId: user.Id, Key: strings.Repeat("r", 32), Status: common.TokenStatusEnabled, RemainQuota: 10000, ExpiredTime: -1, Group: "default"}
			require.NoError(t, db.Create(&token).Error)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var attempt model.TaskCreateAttempt
				if !assert.NoError(t, db.First(&attempt).Error) {
					w.WriteHeader(500)
					return
				}
				assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
				assert.Equal(t, 700, attempt.HeldQuota)
				_, err := io.Copy(io.Discard, r.Body)
				assert.NoError(t, err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			providerModel := "doubao-seedance-2-0-260128"
			if tc.protocol == kitdto.VideoUpstreamProtocolModelArkV3CMCC {
				providerModel = model.CMCCSeedance20ProviderModel
			}
			mapping, err := common.Marshal(map[string]string{"customer-video": providerModel})
			require.NoError(t, err)
			channel := model.Channel{Id: 8191, Type: constant.ChannelTypeSeedanceLink, Name: "rejection-fixture", Key: "provider-fixture-key", BaseURL: &server.URL, Status: common.ChannelStatusEnabled, Models: "customer-video", Group: "default", ModelMapping: common.GetPointer(string(mapping))}
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: tc.protocol})
			require.NoError(t, db.Create(&channel).Error)
			engine := gin.New()
			engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(), middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"480p"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			engine.ServeHTTP(recorder, request)
			require.Equal(t, int32(1), calls.Load(), recorder.Body.String())
			var attempt model.TaskCreateAttempt
			require.NoError(t, db.First(&attempt).Error)
			var count int64
			require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
			assert.Zero(t, count, "no trusted provider task ID means no Task")
			var finalUser model.User
			var finalToken model.Token
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			require.NoError(t, db.First(&finalToken, token.Id).Error)
			if tc.rejected {
				wantStatus := tc.status
				if tc.status == http.StatusUnauthorized {
					wantStatus = http.StatusBadGateway
					assert.Contains(t, recorder.Body.String(), "upstream_auth_error")
				}
				assert.Equal(t, wantStatus, recorder.Code, recorder.Body.String())
				assert.NotContains(t, recorder.Body.String(), "create_outcome_unknown")
				assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
				assert.Equal(t, 10000, finalUser.Quota)
				assert.Equal(t, 10000, finalToken.RemainQuota)
				assert.Zero(t, finalToken.UsedQuota)
				_, err := model.ReleaseTaskCreateAttemptHold(attempt.ID, model.TaskCreateAttemptRejected)
				require.NoError(t, err)
				require.NoError(t, db.First(&finalUser, user.Id).Error)
				require.NoError(t, db.First(&finalToken, token.Id).Error)
				assert.Equal(t, 10000, finalUser.Quota, "repeated release must not credit twice")
				assert.Equal(t, 10000, finalToken.RemainQuota)
				assert.Zero(t, finalToken.UsedQuota)
			} else {
				assert.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), "create_outcome_unknown")
				assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
				assert.Equal(t, 9300, finalUser.Quota)
				assert.Equal(t, 9300, finalToken.RemainQuota)
			}
		})
	}
}
