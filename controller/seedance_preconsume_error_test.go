package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePreconsumeReportsFundingFailureBeforeProvider(t *testing.T) {
	const hold = 4_733_400 // Configured 920000 tokens × $10.29/M × 500000 quota/USD.
	for _, tc := range []struct {
		name, preference, reason string
		wallet, token            int
		success                  bool
	}{
		{"wallet insufficient", "wallet_only", "insufficient_quota", 2_884_180, seedanceFundsInitialQuota, false},
		{"token insufficient", "wallet_only", "insufficient_quota", seedanceFundsInitialQuota, hold - 1, false},
		{"subscription unavailable", "subscription_only", "subscription_unavailable", seedanceFundsInitialQuota, seedanceFundsInitialQuota, false},
		{"exact configured budget available", "wallet_only", "", hold, hold, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			require.NoError(t, fx.db.AutoMigrate(&model.ErrorEvent{}))
			oldQuotaPerUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 500_000
			t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr":           `{"customer-video":"tier(\"base\", u(\"tokens\") * 10.29 / 1000000)"}`,
				"task_billing_setting.preconsume_tokens": `{"customer-video":920000}`,
				"group_ratio_setting.group_ratio":        `{"default":1}`,
				"group_ratio_setting.group_group_ratio":  `{}`,
			}))
			user := model.User{}
			user.SetSetting(kitdto.UserSetting{BillingPreference: tc.preference})
			require.NoError(t, fx.db.Model(&model.User{}).Where("id = ?", fx.userID).
				Updates(map[string]any{"quota": tc.wallet, "setting": user.Setting}).Error)
			require.NoError(t, fx.db.Model(&model.Token{}).Where("user_id = ?", fx.userID).
				Update("remain_quota", tc.token).Error)

			capture := logtest.New(t)
			t.Cleanup(func() {
				require.Eventually(t, func() bool {
					health := capture.Health()
					return health.Accepted == health.Persisted+health.PersistFailed
				}, time.Second, time.Millisecond, "finish event persistence before restoring the fixture database")
				assert.Zero(t, capture.Health().PersistFailed)
				var events []model.ErrorEvent
				require.NoError(t, fx.db.Find(&events).Error)
				if tc.success {
					assert.Empty(t, events)
					return
				}
				require.Len(t, events, 1)
				assert.Equal(t, "billing_preconsume", events[0].Stage)
				assert.Equal(t, tc.reason, events[0].Reason)
				assert.Equal(t, "insufficient_quota", events[0].PublicCode)
				assert.Equal(t, "preconsume-test", events[0].RequestId)
			})
			engine := gin.New()
			engine.Use(capture.Middleware(), middleware.RouteTag("relay"), func(c *gin.Context) {
				c.Set(common.RequestIdKey, "preconsume-test")
				c.Next()
			})
			engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(),
				middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(),
				middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
			request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks",
				bytes.NewBufferString(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":4,"resolution":"720p","ratio":"21:9"}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			var attempt model.TaskCreateAttempt
			require.NoError(t, fx.db.First(&attempt).Error)
			var snapshot struct {
				Quota int `json:"quota"`
			}
			require.NoError(t, common.Unmarshal(attempt.BillingSnapshot, &snapshot))
			assert.Equal(t, hold, snapshot.Quota, "the administrator budget still determines the hold")
			var tasks int64
			require.NoError(t, fx.db.Model(&model.Task{}).Count(&tasks).Error)
			remain, used := fx.tokenQuota()
			if tc.success {
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.EqualValues(t, 1, fx.createCalls.Load())
				assert.EqualValues(t, 1, tasks)
				assert.Equal(t, model.TaskCreateAttemptComplete, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempt.BillingHoldState)
				assert.Equal(t, hold, attempt.HeldQuota)
				assert.Zero(t, fx.userQuota())
				assert.Zero(t, remain)
				assert.Equal(t, hold, used)
				assert.Empty(t, capture.String())
				return
			}

			require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
			var body struct {
				Error struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"request_id"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
			assert.Equal(t, "insufficient_quota", body.Error.Code)
			assert.Contains(t, body.Error.Message, "Insufficient quota")
			assert.Equal(t, "preconsume-test", body.Error.RequestID)
			assert.Zero(t, fx.createCalls.Load())
			assert.Zero(t, tasks)
			assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
			assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
			assert.Zero(t, attempt.HeldQuota)
			assert.Equal(t, tc.wallet, fx.userQuota())
			assert.Equal(t, tc.token, remain)
			assert.Zero(t, used)
			log := capture.String()
			assert.Equal(t, 1, strings.Count(log, "event=authenticated_api_client_error"))
			assert.Contains(t, log, "stage=billing_preconsume")
			assert.Contains(t, log, "reason="+tc.reason)
			assert.Contains(t, log, "public_code=insufficient_quota")
			assert.Contains(t, log, "model=customer-video")
			assert.Contains(t, log, "channel_id=9101")
			assert.Contains(t, log, "protocol=feicai_videos_v1")
			for _, secret := range []string{strings.Repeat("f", 32), "provider-fixture-key", "A landscape", "2884180"} {
				assert.NotContains(t, log, secret)
				assert.NotContains(t, response.Body.String(), secret)
			}
		})
	}
}
