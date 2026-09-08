package controller

import (
	"bytes"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHostedVideoCreationPersistsFactsAndRejectsBeforeHold(t *testing.T) {
	for _, outcome := range []string{"accepted", "delete_after_accept", "ambiguous", "other_user", "deleted", "missing", "video_slot", "audio_slot", "non_hosted", "object_missing", "store_unavailable", "location_changed", "direct_url"} {
		t.Run(outcome, func(t *testing.T) {
			service.InitHttpClient()
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.TaskCreateAttempt{}, &model.TaskCreateIdempotency{}, &model.UserSubscription{}, &model.FunCloudHostedAsset{}, &model.FunCloudHostedAssetGroup{}))
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

			var providerCalls, storageCalls atomic.Int32
			objectStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				storageCalls.Add(1)
				assert.Equal(t, http.MethodHead, r.Method)
				assert.Equal(t, "/images/prod/assets/hosted.png", r.URL.Path)
				if outcome == "object_missing" {
					w.WriteHeader(404)
					return
				}
				if outcome == "store_unavailable" {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Length", "12")
			}))
			t.Cleanup(objectStore.Close)
			storageConfig := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: objectStore.URL, Bucket: "images", Prefix: "prod", AccountName: "account", Region: "us-east-1", Credential: "fixture-secret", Revision: outcome}
			encoded, err := common.Marshal(storageConfig)
			require.NoError(t, err)
			model.NotifyObjectStorageSettingUpdate(string(encoded))
			t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
			asset := model.FunCloudHostedAsset{ID: "fhas_fixture", UserID: user.Id, Name: "fixture", Model: "customer-funcloud", ObjectKey: "assets/hosted.png", MimeType: "image/png", SizeBytes: 12, Status: model.FunCloudHostedAssetStatusReady,
				StorageLocation: model.FunCloudHostedStorageLocation{Backend: "s3", Endpoint: objectStore.URL, Bucket: "images", Prefix: "prod"}}
			if outcome == "other_user" {
				asset.UserID++
			}
			if outcome == "deleted" {
				asset.Status = model.FunCloudHostedAssetStatusDeleted
			}
			if outcome == "location_changed" {
				asset.StorageLocation.Bucket = "old-bucket"
			}
			if outcome != "missing" {
				require.NoError(t, db.Create(&asset).Error)
			}
			var sendingFacts []model.TaskHostedMediaFact
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				providerCalls.Add(1)
				var attempt model.TaskCreateAttempt
				if !assert.NoError(t, db.First(&attempt).Error) {
					w.WriteHeader(500)
					return
				}
				assert.Equal(t, model.TaskCreateAttemptSending, attempt.Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempt.BillingHoldState)
				assert.Equal(t, 700, attempt.HeldQuota)
				var snapshot struct {
					PrivateData model.TaskPrivateData `json:"private_data"`
				}
				assert.NoError(t, common.Unmarshal(attempt.RecoverySnapshot, &snapshot))
				sendingFacts = snapshot.PrivateData.HostedMedia
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				var payload struct {
					Content []struct {
						ImageURL *struct {
							URL string `json:"url"`
						} `json:"image_url"`
					} `json:"content"`
					RealPersonMode bool `json:"real_person_mode"`
				}
				if !assert.NoError(t, common.Unmarshal(body, &payload)) {
					w.WriteHeader(500)
					return
				}
				assert.True(t, payload.RealPersonMode)
				assert.NotContains(t, string(body), "asset://fhas_")
				if outcome == "direct_url" {
					assert.Empty(t, sendingFacts)
					assert.Contains(t, string(body), "https://source.example/image.png")
				} else {
					if assert.Len(t, sendingFacts, 1) {
						assert.Equal(t, asset.StorageLocation, sendingFacts[0].StorageLocation)
						assert.Equal(t, asset.ObjectKey, sendingFacts[0].ObjectKey)
					}
					signed, err := url.Parse(payload.Content[1].ImageURL.URL)
					assert.NoError(t, err)
					assert.Equal(t, "/images/prod/assets/hosted.png", signed.Path)
					assert.Equal(t, "86400", signed.Query().Get("X-Amz-Expires"))
				}
				if outcome == "delete_after_accept" {
					_, err := model.MarkFunCloudHostedAssetDeleted(user.Id, asset.ID)
					assert.NoError(t, err)
				}
				w.Header().Set("Content-Type", "application/json")
				if outcome == "ambiguous" {
					_, _ = w.Write([]byte(`{"unexpected":true}`))
					return
				}
				_, _ = w.Write([]byte(`{"id":"hosted-provider-task"}`))
			}))
			t.Cleanup(provider.Close)
			channel := model.Channel{Id: 8190, Type: constant.ChannelTypeSeedanceLink, Name: "fixture", Key: "provider-fixture-key", BaseURL: &provider.URL, Status: common.ChannelStatusEnabled, Models: "customer-funcloud", Group: "default", ModelMapping: common.GetPointer(`{"customer-funcloud":"seedance-2-0"}`)}
			settings := dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted}
			if outcome == "non_hosted" {
				settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
			}
			channel.SetOtherSettings(settings)
			require.NoError(t, db.Create(&channel).Error)
			engine := gin.New()
			engine.POST("/api/v3/contents/generations/tasks", middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"), middleware.TaskCreateResponseContract(), middleware.ModelArkVideoCreateConvert(), middleware.ResolveSeedanceChannel(), RelayTask)
			mediaType, role, reference := "image_url", "reference_image", "asset://fhas_fixture"
			if outcome == "video_slot" {
				mediaType, role = "video_url", "reference_video"
			}
			if outcome == "audio_slot" {
				mediaType, role = "audio_url", "reference_audio"
			}
			if outcome == "direct_url" {
				reference = "https://source.example/image.png"
			}
			payload := map[string]any{"model": "customer-funcloud", "duration": 4, "content": []any{map[string]any{"type": "text", "text": "Landscape"}, map[string]any{"type": mediaType, "role": role, mediaType: map[string]any{"url": reference}}}}
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer sk-"+token.Key)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var tasks []model.Task
			require.NoError(t, db.Find(&tasks).Error)
			var attempts []model.TaskCreateAttempt
			require.NoError(t, db.Find(&attempts).Error)
			var finalUser model.User
			require.NoError(t, db.First(&finalUser, user.Id).Error)
			if outcome == "accepted" || outcome == "delete_after_accept" || outcome == "direct_url" {
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Equal(t, int32(1), providerCalls.Load())
				require.Len(t, tasks, 1)
				assert.Equal(t, sendingFacts, tasks[0].PrivateData.HostedMedia)
				assert.Equal(t, 9300, finalUser.Quota)
			} else if outcome == "ambiguous" {
				require.Equal(t, int32(1), providerCalls.Load())
				require.Empty(t, tasks)
				require.Len(t, attempts, 1)
				assert.Equal(t, model.TaskCreateAttemptUnknown, attempts[0].Status)
				assert.Equal(t, model.TaskCreateAttemptBillingHeld, attempts[0].BillingHoldState)
				assert.Equal(t, 9300, finalUser.Quota)
				assert.NotContains(t, string(attempts[0].RecoverySnapshot), "X-Amz-")
			} else {
				if outcome == "object_missing" || outcome == "store_unavailable" || outcome == "location_changed" {
					assert.Equal(t, 503, response.Code, response.Body.String())
				} else {
					assert.Equal(t, 400, response.Code, response.Body.String())
				}
				assert.Zero(t, providerCalls.Load())
				assert.Empty(t, tasks)
				assert.Empty(t, attempts)
				assert.Equal(t, 10000, finalUser.Quota)
				var finalToken model.Token
				require.NoError(t, db.First(&finalToken, token.Id).Error)
				assert.Equal(t, 10000, finalToken.RemainQuota)
			}
			if outcome == "direct_url" {
				assert.Zero(t, storageCalls.Load())
			}
			assert.NotContains(t, response.Body.String(), "X-Amz-")
			assert.NotContains(t, response.Body.String(), "assets/hosted.png")
			if len(tasks) > 0 {
				encoded, err := common.Marshal(tasks[0].PrivateData)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), "X-Amz-")
			}
		})
	}
}
