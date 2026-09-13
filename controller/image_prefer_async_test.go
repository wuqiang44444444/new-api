package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGPTImage2SynchronousGenerationAndEditPreserveBilling(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefer string
		key    string
		edit   bool
		quota  int
	}{
		{name: "without_preference", quota: 100000},
		{name: "multipart_edit", edit: true, quota: 100000},
		{name: "insufficient_funds", quota: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			oldDB, oldLog := model.DB, model.LOG_DB
			oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
			oldRedis, oldBatch, oldConsume, oldCount := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken
			oldPrice, oldGroup := ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
			t.Cleanup(func() {
				model.DB, model.LOG_DB = oldDB, oldLog
				common.SetDatabaseTypes(oldMainType, oldLogType)
				common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken = oldRedis, oldBatch, oldConsume, oldCount
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
				require.NoError(t, sqlDB.Close())
			})
			model.DB = db
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken = false, false, false, false
			t.Setenv("LOG_SQL_DSN", "")
			require.NoError(t, model.InitLogDB())
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.TaskBillingDelivery{}, &model.QuotaData{}, &model.TaskCreateIdempotency{}, &model.Log{}))
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-2":0.04}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			withTieredBillingConfig(t, map[string]string{}, map[string]string{})
			service.InitHttpClient()
			user := model.User{Id: 8761, Username: "image-prefer-fixture", Quota: tc.quota, Group: "default", Status: common.UserStatusEnabled}
			token := model.Token{Id: 8761, UserId: user.Id, Key: strings.Repeat("p", 32), RemainQuota: tc.quota, Status: common.TokenStatusEnabled, ExpiredTime: -1}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.Create(&token).Error)
			const charge = 20000
			const response = `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
			var calls atomic.Int32
			path := "/v1/images/generations"
			if tc.edit {
				path = "/v1/images/edits"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := int(calls.Add(1))
				var heldUser model.User
				var heldToken model.Token
				assert.NoError(t, db.First(&heldUser, user.Id).Error)
				assert.NoError(t, db.First(&heldToken, token.Id).Error)
				assert.Equal(t, tc.quota-call*charge, heldUser.Quota, "wallet must be charged before provider POST")
				assert.Equal(t, tc.quota-call*charge, heldToken.RemainQuota, "token must be charged before provider POST")
				assert.Equal(t, path, r.URL.Path)
				if tc.edit {
					if assert.NoError(t, r.ParseMultipartForm(1<<20)) {
						defer r.MultipartForm.RemoveAll()
						assert.Equal(t, "gpt-image-2", r.FormValue("model"))
						assert.Len(t, r.MultipartForm.File["image"], 1)
					}
				} else {
					body, readErr := io.ReadAll(r.Body)
					assert.NoError(t, readErr)
					assert.JSONEq(t, `{"model":"gpt-image-2","prompt":"a cat","n":1}`, string(body))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			}))
			t.Cleanup(server.Close)
			channel := model.Channel{Id: 8761, Type: constant.ChannelTypeOpenAI, Name: "image-prefer", Key: "fixture-key", BaseURL: &server.URL, Status: common.ChannelStatusEnabled}
			require.NoError(t, db.Create(&channel).Error)
			engine := gin.New()
			engine.POST(path, func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyUserQuota, tc.quota)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-image-2"))
				c.Next()
			}, middleware.ImageCreateIdempotency(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
			requests := 1
			if tc.key != "" {
				requests = 2 // A successful native response must not leave a pending task claim.
			}
			for i := 0; i < requests; i++ {
				body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"a cat","n":1}`)
				contentType := "application/json"
				if tc.edit {
					body.Reset()
					writer := multipart.NewWriter(body)
					require.NoError(t, writer.WriteField("model", "gpt-image-2"))
					require.NoError(t, writer.WriteField("prompt", "a cat"))
					require.NoError(t, writer.WriteField("n", "1"))
					part, err := writer.CreateFormFile("image", "input.png")
					require.NoError(t, err)
					_, err = part.Write([]byte("image fixture"))
					require.NoError(t, err)
					require.NoError(t, writer.Close())
					contentType = writer.FormDataContentType()
				}
				request := httptest.NewRequest(http.MethodPost, path, body)
				request.Header.Set("Content-Type", contentType)
				request.Header.Set("Prefer", tc.prefer)
				request.Header.Set("Idempotency-Key", tc.key)
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				if tc.quota < charge {
					assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
					assert.Zero(t, calls.Load())
				} else {
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					assert.JSONEq(t, response, recorder.Body.String())
					assert.Empty(t, recorder.Header().Get("Location"))
				}
			}
			var finalUser model.User
			var finalToken model.Token
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			require.NoError(t, db.First(&finalToken, token.Id).Error)
			assert.Equal(t, tc.quota-int(calls.Load())*charge, finalUser.Quota)
			assert.Equal(t, finalUser.Quota, finalToken.RemainQuota)
			assert.Equal(t, int(calls.Load())*charge, finalToken.UsedQuota)
			for _, record := range []any{&model.Task{}, &model.TaskCreateIdempotency{}} {
				var count int64
				require.NoError(t, db.Model(record).Count(&count).Error)
				assert.Zero(t, count)
			}
		})
	}
}
