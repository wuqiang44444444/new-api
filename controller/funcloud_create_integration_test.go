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

func TestFunCloudCreateRouteCommitsHoldBeforeProviderBytes(t *testing.T) {
	for _, outcome := range []string{"accepted", "ambiguous", "copyright", "sensitive"} {
		t.Run(outcome, func(t *testing.T) {
			service.InitHttpClient()
			policyMessage := ""
			if outcome == "copyright" {
				policyMessage = "The request failed because the output video may be related to copyright restrictions."
			}
			if outcome == "sensitive" {
				policyMessage = "输入内容可能包含敏感信息，请检查后重试。"
			}
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
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
				"billing_setting.billing_expr": `{"customer-funcloud":"tier(\"fixed\", 1400)"}`,
			}))
			user := model.User{Id: 8190, Username: "funcloud-integration", Quota: 10000, Status: common.UserStatusEnabled, Group: "default"}
			user.SetSetting(kitdto.UserSetting{BillingPreference: "wallet_only"})
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{Id: 8190, UserId: user.Id, Key: strings.Repeat("f", 32), Status: common.TokenStatusEnabled, RemainQuota: 10000, ExpiredTime: -1, Group: "default"}
			require.NoError(t, db.Create(&token).Error)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				// This is a separate DB read on the receiving side, before reading the POST body.
				var attempt model.TaskCreateAttempt
				if !assert.NoError(t, db.First(&attempt).Error) {
					w.WriteHeader(500)
					return
				}
				assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
				assert.Equal(t, 700, attempt.HeldQuota)
				var charged model.User
				assert.NoError(t, db.First(&charged, user.Id).Error)
				assert.Equal(t, 9300, charged.Quota)
				var heldToken model.Token
				assert.NoError(t, db.First(&heldToken, token.Id).Error)
				assert.Equal(t, 9300, heldToken.RemainQuota)
				var count int64
				assert.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
				assert.Zero(t, count)
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v3/contents/generations/tasks", r.URL.Path)
				assert.Contains(t, string(body), `"model":"seedance-2-0"`)
				assert.Contains(t, string(body), "asset://opaque-reference")
				w.Header().Set("Content-Type", "application/json")
				if policyMessage != "" {
					w.WriteHeader(http.StatusForbidden)
					response, err := common.Marshal(map[string]any{"error": map[string]any{"code": "ContentPolicyViolation", "message": policyMessage}})
					assert.NoError(t, err)
					_, _ = w.Write(response)
					return
				}
				if outcome == "ambiguous" {
					_, _ = w.Write([]byte(`{"unexpected":"no-trusted-task-id"}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"provider-video-123"}`))
			}))
			defer server.Close()
			channel := model.Channel{Id: 8190, Type: constant.ChannelTypeSeedanceLink, Name: "fixture", Key: "provider-fixture-key", BaseURL: &server.URL, Status: common.ChannelStatusEnabled, Models: "customer-funcloud", Group: "default", ModelMapping: common.GetPointer(`{"customer-funcloud":"seedance-2-0"}`)}
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3})
			require.NoError(t, db.Create(&channel).Error)
			engine := gin.New()
			engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"),
				middleware.TaskCreateResponseContract(), middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(`{"model":"customer-funcloud","content":[{"type":"text","text":"A landscape"},{"type":"image_url","role":"reference_image","image_url":{"url":"asset://opaque-reference"}}],"duration":4}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-"+token.Key)
			engine.ServeHTTP(recorder, req)
			require.Equal(t, int32(1), calls.Load(), recorder.Body.String())
			var attempt model.TaskCreateAttempt
			require.NoError(t, db.First(&attempt).Error)
			var tasks []model.Task
			require.NoError(t, db.Find(&tasks).Error)
			if outcome == "accepted" {
				require.Equal(t, 200, recorder.Code, recorder.Body.String())
				require.Len(t, tasks, 1)
				assert.Equal(t, attempt.PublicTaskID, tasks[0].TaskID)
				assert.Equal(t, "provider-video-123", tasks[0].GetUpstreamTaskID())
				assert.Equal(t, 700, tasks[0].Quota)
				assert.NotContains(t, recorder.Body.String(), "provider-video-123")
				assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempt.BillingHoldState)
			} else if policyMessage != "" {
				assert.Equal(t, http.StatusForbidden, recorder.Code)
				var response struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				assert.Equal(t, "ContentPolicyViolation", response.Error.Code)
				assert.Contains(t, response.Error.Message, policyMessage)
				assert.NotContains(t, response.Error.Message, "HTTP 403")
				assert.NotContains(t, response.Error.Message, "credentials")
				assert.Empty(t, tasks)
				assert.Equal(t, model.TaskCreateAttemptRejected, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingReleased, attempt.BillingHoldState)
			} else {
				assert.GreaterOrEqual(t, recorder.Code, 400)
				assert.Empty(t, tasks)
				assert.Equal(t, model.TaskCreateAttemptUnknown, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
			}
			var finalUser model.User
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			if policyMessage != "" {
				assert.Equal(t, 10000, finalUser.Quota)
			} else {
				assert.Equal(t, 9300, finalUser.Quota)
			}
		})
	}
}
