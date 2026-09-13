package controller

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func nativeImageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLog := model.DB, model.LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldBatch, oldConsume, oldCount := common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken
	oldPrice, oldGroup := ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
	oldExecutor, oldResume := service.ImageTaskExecuteFunc, service.ImageTaskResumePollFunc
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLog
		common.SetDatabaseTypes(oldMain, oldLogType)
		common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken = oldRedis, oldBatch, oldConsume, oldCount
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroup))
		service.ImageTaskExecuteFunc, service.ImageTaskResumePollFunc = oldExecutor, oldResume
		model.NotifyObjectStorageSettingUpdate("")
		require.NoError(t, sqlDB.Close())
	})
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.BatchUpdateEnabled, common.LogConsumeEnabled, constant.CountToken = false, false, false, false
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Task{}, &model.TaskBillingDelivery{}, &model.QuotaData{}, &model.TaskCreateIdempotency{}, &model.ImageTaskSlot{}, &model.Log{}, &model.UserSubscription{}, &model.SubscriptionPlan{}, &model.SubscriptionPreConsumeRecord{}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-2":0.04}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	withTieredBillingConfig(t, map[string]string{}, map[string]string{})
	service.InitHttpClient()
	service.ImageTaskExecuteFunc, service.ImageTaskResumePollFunc = relay.ExecuteImageTask, relay.ResumeImageTaskPoll
	objects := map[string][]byte{}
	var mu sync.Mutex
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			objects[r.URL.Path] = data
			w.WriteHeader(200)
		case http.MethodHead, http.MethodGet:
			data, exists := objects[r.URL.Path]
			if !exists {
				w.WriteHeader(404)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
			}
		default:
			w.WriteHeader(405)
		}
	}))
	t.Cleanup(store.Close)
	config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", AccountName: "account", Region: "us-east-1", Credential: "storage-fixture", Revision: t.Name()})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(config))
	return db
}

func TestNativeImageAsyncAcceptanceExecutionAndQuery(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure} {
		for _, operation := range []string{"generations", "edits"} {
			for _, stream := range []bool{false, true} {
				for _, wireMode := range []string{"converted", "passthrough", "override", "stream_override"} {
					if wireMode != "converted" && (stream || (operation != "generations" && wireMode != "stream_override")) {
						continue
					}
					t.Run(strconv.Itoa(channelType)+"/"+operation+"/stream="+strconv.FormatBool(stream)+"/"+wireMode, func(t *testing.T) {
						db := nativeImageTestDB(t)
						user := model.User{Id: 8911, Username: "native-image", Quota: 100000, Group: "default", Status: common.UserStatusEnabled}
						token := model.Token{Id: 8911, UserId: user.Id, Key: strings.Repeat("n", 32), RemainQuota: 100000, Status: common.TokenStatusEnabled, ExpiredTime: -1}
						require.NoError(t, db.Create(&user).Error)
						require.NoError(t, db.Create(&token).Error)
						var calls atomic.Int32
						provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							var held model.User
							assert.NoError(t, db.First(&held, user.Id).Error)
							if !stream {
								assert.Equal(t, 60000, held.Quota, "n=2 is held before provider POST")
							}
							expectedPath := "/v1/images/" + operation
							if channelType == constant.ChannelTypeAzure {
								expectedPath = "/openai/deployments/deploy.v2/images/" + operation
								assert.Equal(t, "2025-04-01-preview", r.URL.Query().Get("api-version"))
								assert.Equal(t, "native-secret", r.Header.Get("api-key"))
							} else {
								assert.Equal(t, "Bearer native-secret", r.Header.Get("Authorization"))
							}
							assert.Equal(t, expectedPath, r.URL.Path)
							assert.Equal(t, "frozen-header", r.Header.Get("X-Frozen"))
							if operation == "edits" && wireMode != "stream_override" {
								if assert.NoError(t, r.ParseMultipartForm(1<<20)) {
									defer r.MultipartForm.RemoveAll()
									assert.Equal(t, "deploy.v2", r.FormValue("model"))
									assert.Len(t, r.MultipartForm.File["image[]"], 2)
									assert.Len(t, r.MultipartForm.File["mask"], 1)
									assert.Equal(t, []string{"first", "second"}, r.MultipartForm.Value["user"])
									assert.Equal(t, "0", r.FormValue("output_compression"))
								}
							} else {
								var body map[string]any
								assert.NoError(t, common.DecodeJson(r.Body, &body))
								if wireMode == "passthrough" {
									assert.Equal(t, "gpt-image-2", body["model"])
								} else {
									assert.Equal(t, "deploy.v2", body["model"])
								}
								if wireMode == "override" {
									assert.Equal(t, "high", body["quality"])
								}
								if wireMode == "stream_override" {
									assert.Equal(t, true, body["stream"])
									if operation == "edits" {
										assert.Equal(t, "data:image/png;base64,aW1hZ2U=", body["image"])
									}
								}
								assert.Equal(t, float64(0), body["output_compression"])
								assert.NotContains(t, body, "response_format", "absence must stay absent for GPT image")
							}
							if wireMode == "stream_override" {
								w.Header().Set("Content-Type", "text/event-stream")
								event := "image_generation.completed"
								if operation == "edits" {
									event = "image_edit.completed"
								}
								_, _ = io.WriteString(w, "data: {\"type\":\""+event+"\",\"b64_json\":\"aW1hZ2U=\",\"usage\":{\"input_tokens\":12,\"output_tokens\":20,\"total_tokens\":32,\"input_tokens_details\":{\"cached_tokens\":2,\"image_tokens\":4,\"text_tokens\":8}}}\n\ndata: [DONE]\n\n")
								return
							}
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"created":1700000000,"data":[{"b64_json":"aW1hZ2U="}],"usage":{"input_tokens":12,"output_tokens":20,"total_tokens":32,"input_tokens_details":{"cached_tokens":2,"image_tokens":4,"text_tokens":8}}}`)
						}))
						t.Cleanup(provider.Close)
						mapping := `{"gpt-image-2":"deploy.v2"}`
						headers := `{"X-Frozen":"{client_header:X-Freeze}"}`
						version := "2025-04-01-preview"
						channel := model.Channel{Id: 8911, Type: channelType, Name: "native-image", Key: "native-secret", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, ModelMapping: &mapping, HeaderOverride: &headers, Other: version, CreatedTime: constant.AzureNoRemoveDotTime + 1}
						if wireMode == "passthrough" {
							setting, err := common.Marshal(dto.ChannelSettings{PassThroughBodyEnabled: true})
							require.NoError(t, err)
							v := string(setting)
							channel.Setting = &v
						}
						if wireMode == "override" {
							override := `{"quality":"high"}`
							channel.ParamOverride = &override
						}
						if wireMode == "stream_override" {
							override := `{"stream":true}`
							channel.ParamOverride = &override
						}
						require.NoError(t, db.Create(&channel).Error)
						engine := gin.New()
						auth := func(c *gin.Context) {
							common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
							common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
							common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
							common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
							common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
							common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
							common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
							common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
							require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-image-2"))
							c.Next()
						}
						path := "/v1/images/" + operation
						engine.POST(path, auth, middleware.ImageCreateIdempotency(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
						engine.GET("/v1/tasks/:key", auth, GetTask)
						var taskID string
						for attempt := 0; attempt < 2; attempt++ {
							body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"draw","n":2,"output_compression":0,"stream":` + strconv.FormatBool(stream) + `}`)
							contentType := "application/json"
							if operation == "edits" && wireMode == "stream_override" {
								body = bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"draw","n":2,"output_compression":0,"stream":false,"image":"data:image/png;base64,aW1hZ2U="}`)
							}
							if operation == "edits" && wireMode != "stream_override" {
								body.Reset()
								writer := multipart.NewWriter(body)
								for _, pair := range [][2]string{{"model", "gpt-image-2"}, {"prompt", "draw"}, {"n", "2"}, {"stream", strconv.FormatBool(stream)}, {"user", "first"}, {"user", "second"}, {"output_compression", "0"}} {
									require.NoError(t, writer.WriteField(pair[0], pair[1]))
								}
								for _, field := range []string{"image", "image", "mask"} {
									part, err := writer.CreateFormFile(field, field+".png")
									require.NoError(t, err)
									_, err = part.Write([]byte("image-fixture"))
									require.NoError(t, err)
								}
								require.NoError(t, writer.Close())
								contentType = writer.FormDataContentType()
							}
							request := httptest.NewRequest(http.MethodPost, path, body)
							request.Header.Set("Content-Type", contentType)
							request.Header.Set("Prefer", "respond-async")
							request.Header.Set("Idempotency-Key", "native-key")
							request.Header.Set("X-Freeze", "frozen-header")
							recorder := httptest.NewRecorder()
							engine.ServeHTTP(recorder, request)
							if stream {
								require.Equal(t, 200, recorder.Code, recorder.Body.String())
								assert.Contains(t, recorder.Body.String(), "image_generation.completed")
							} else {
								require.Equal(t, 202, recorder.Code, recorder.Body.String())
								var accepted struct {
									ID string `json:"id"`
								}
								require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &accepted))
								if attempt == 0 {
									taskID = accepted.ID
								} else {
									assert.Equal(t, taskID, accepted.ID)
								}
								assert.Equal(t, "/v1/tasks/"+taskID, recorder.Header().Get("Location"))
								assert.Zero(t, calls.Load())
							}
						}
						var count int64
						require.NoError(t, db.Model(&model.TaskCreateIdempotency{}).Count(&count).Error)
						if stream {
							assert.Zero(t, count)
							require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
							assert.Zero(t, count)
							assert.EqualValues(t, 2, calls.Load())
							return
						}
						assert.EqualValues(t, 1, count)
						var task model.Task
						require.NoError(t, db.First(&task, "task_id = ?", taskID).Error)
						private, err := common.Marshal(task.PrivateData)
						require.NoError(t, err)
						assert.NotContains(t, string(private), "native-secret")
						assert.NotContains(t, string(private), "image-fixture")
						assert.Empty(t, task.PrivateData.ImageTask.ChannelKey)
						// Restart reads persisted facts; subsequent channel changes cannot alter send.
						require.NoError(t, db.Model(&channel).Updates(map[string]any{"key": "rotated", "base_url": "http://invalid.invalid", "model_mapping": "{}"}).Error)
						service.RunImageTaskWorkerOnce(context.Background())
						service.RunImageTaskWorkerOnce(context.Background())
						require.NoError(t, db.First(&task, task.ID).Error)
						assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
						assert.EqualValues(t, 1, calls.Load())
						assert.Equal(t, 20000, task.Quota, "settle returned n=1 instead of requested n=2")
						require.NotNil(t, task.PrivateData.ImageTask.Usage)
						assert.Equal(t, 12, task.PrivateData.ImageTask.Usage.PromptTokens)
						assert.Equal(t, 2, task.PrivateData.ImageTask.Usage.PromptTokensDetails.CachedTokens)
						require.NoError(t, db.First(&user, user.Id).Error)
						require.NoError(t, db.First(&token, token.Id).Error)
						assert.Equal(t, 80000, user.Quota)
						assert.Equal(t, 80000, token.RemainQuota)
						query := httptest.NewRecorder()
						engine.ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID, nil))
						require.Equal(t, 200, query.Code, query.Body.String())
						assert.Contains(t, query.Body.String(), `"status":"succeeded"`)
						assert.Contains(t, query.Body.String(), `"status":"available"`)
						assert.NotContains(t, query.Body.String(), "native-secret")
						assert.NotContains(t, query.Body.String(), "deploy.v2")
						token.Id++
						hidden := httptest.NewRecorder()
						engine.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/v1/tasks/"+taskID, nil))
						assert.Equal(t, 404, hidden.Code)
					})
				}
			}
		}
	}
}

func TestNativeImageAsyncUnknownNeverResendsOrRefunds(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		status                      int
		body                        string
		want                        model.TaskStatus
		quota                       int
		contentType                 string
		rotateKey, preparationError bool
	}{
		{name: "definite_rejection", status: 400, body: `{"error":{"message":"rejected"}}`, want: model.TaskStatusFailure, quota: 100000},
		{name: "server_error", status: 502, body: `{}`, want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "rate_limit", status: 429, body: `{}`, want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "malformed_success", status: 200, body: `{"data":[`, want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "stream_error_after_image", status: 200, contentType: "text/event-stream", body: "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"aW1hZ2U=\"}\n\ndata: {\"type\":\"error\"}\n\n", want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "stream_partial_only", status: 200, contentType: "text/event-stream", body: "data: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"aW1hZ2U=\"}\n\ndata: [DONE]\n\n", want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "changed_root_key", rotateKey: true, want: model.TaskStatusReconciliationRequired, quota: 80000},
		{name: "preparation_error", preparationError: true, quota: 100000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := nativeImageTestDB(t)
			user := model.User{Id: 8941, Username: "native-outcome", Quota: 100000}
			require.NoError(t, db.Create(&user).Error)
			token := model.Token{Id: 8941, UserId: user.Id, Key: strings.Repeat("z", 32), RemainQuota: 100000}
			require.NoError(t, db.Create(&token).Error)
			var calls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(provider.Close)
			channel := model.Channel{Id: 8941, Type: constant.ChannelTypeOpenAI, Key: "fixture", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled}
			var diagnostics bytes.Buffer
			if tc.preparationError {
				override := `{"operations":[{"path":"prompt","mode":"unsupported-private-mode","value":"private-body"}]}`
				channel.ParamOverride = &override
				oldWriter := gin.DefaultErrorWriter
				gin.DefaultErrorWriter = &diagnostics
				t.Cleanup(func() { gin.DefaultErrorWriter = oldWriter })
			}
			require.NoError(t, db.Create(&channel).Error)
			engine := gin.New()
			engine.POST("/v1/images/generations", func(c *gin.Context) {
				c.Set(common.RequestIdKey, "image-preparation-fixture")
				common.SetContextKey(c, constant.ContextKeyUserId, user.Id)
				common.SetContextKey(c, constant.ContextKeyUserQuota, 100000)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				common.SetContextKey(c, constant.ContextKeyTokenId, token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, token.Key)
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &channel, "gpt-image-2"))
				c.Next()
			}, middleware.ImageCreateIdempotency(), func(c *gin.Context) { Relay(c, types.RelayFormatOpenAIImage) })
			request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2","prompt":"draw"}`))
			request.Header.Set("Prefer", "respond-async")
			request.Header.Set("Content-Type", "application/json")
			if tc.preparationError {
				request.Header.Set("Idempotency-Key", "failed-preparation-key")
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if tc.preparationError {
				require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
				assert.Contains(t, diagnostics.String(), "stage=parameter_override")
				assert.Contains(t, diagnostics.String(), "image-preparation-fixture")
				assert.NotContains(t, diagnostics.String(), "unsupported-private-mode")
				assert.NotContains(t, diagnostics.String(), "private-body")
				assert.NotContains(t, recorder.Body.String(), "unsupported-private-mode")
				var count int64
				require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
				assert.Zero(t, count)
				require.NoError(t, db.Model(&model.TaskCreateIdempotency{}).Count(&count).Error)
				assert.Zero(t, count)
				require.NoError(t, db.First(&user, user.Id).Error)
				require.NoError(t, db.First(&token, token.Id).Error)
				assert.Equal(t, 100000, user.Quota)
				assert.Equal(t, 100000, token.RemainQuota)
				assert.Zero(t, calls.Load())
				return
			}
			require.Equal(t, 202, recorder.Code, recorder.Body.String())
			if tc.rotateKey {
				oldKey := common.CryptoSecret
				common.CryptoSecret = "different-worker-test-key"
				t.Cleanup(func() { common.CryptoSecret = oldKey })
			}
			assert.Zero(t, calls.Load())
			service.RunImageTaskWorkerOnce(context.Background())
			service.RunImageTaskWorkerOnce(context.Background())
			var task model.Task
			require.NoError(t, db.First(&task).Error)
			assert.Equal(t, tc.want, task.Status)
			if tc.rotateKey {
				assert.Zero(t, calls.Load(), "unreadable frozen credentials must never be replaced or sent")
			} else {
				assert.EqualValues(t, 1, calls.Load())
			}
			require.NoError(t, db.First(&user, user.Id).Error)
			require.NoError(t, db.First(&token, token.Id).Error)
			assert.Equal(t, tc.quota, user.Quota)
			assert.Equal(t, tc.quota, token.RemainQuota)
		})
	}
}
