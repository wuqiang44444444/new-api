package service_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const seedanceSixTierExpression = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "4k"     ? tier("4k_video",      c * 2.4)
    : param("_task.resolution") == "1080p" ? tier("1080p_video",   c * 4.7)
    :                                       tier("480p720p_video", c * 4.3))
  : (param("_task.resolution") == "4k"     ? tier("4k",            c * 4.0)
    : param("_task.resolution") == "1080p" ? tier("1080p",         c * 7.7)
    :                                       tier("480p720p",       c * 7.0))`

const tokenSavePerSecondExpression = `param("_task.resolution") == "1080p"
  ? tier("1080p", param("_task.duration_seconds") * 364700)
  : param("_task.resolution") == "720p"
    ? (param("_task.input_mode") != "text" && param("_task.control_mode") != "reference"
      ? tier("720p_image", param("_task.duration_seconds") * 146100)
      : tier("720p_text_or_reference", param("_task.duration_seconds") * 146200))
    : tier("480p", param("_task.duration_seconds") * 67900)`

const moxingOverseaTokenExpression = `v1:tier("480p720p", c * 7.0)`

type videoPollingRequest struct {
	Method        string
	Path          string
	Authorization string
}

func TestSeedanceVideoChannelsRespectImplementationSpecificBillingEvidence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resetVideoE2ETables(t)
	loadVideoE2EBillingConfig(t)

	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousQuotaPerUnit := common.QuotaPerUnit
	previousAdaptorFactory := service.GetTaskAdaptorFunc
	common.MemoryCacheEnabled = false
	common.QuotaPerUnit = 500000
	service.InitHttpClient()
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		if platform == constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)) {
			return &doubao.TaskAdaptor{}
		}
		return nil
	}
	t.Cleanup(func() {
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.QuotaPerUnit = previousQuotaPerUnit
		service.GetTaskAdaptorFunc = previousAdaptorFactory
	})

	const (
		actualTokens = 108900
		actualQuota  = 381150
		initialQuota = 20000000
	)

	tests := []struct {
		name                 string
		channelID            int
		modelName            string
		profile              dto.VideoUpstreamProfile
		preConsumeTokens     int
		expectedPreConsume   int
		queryPathTemplate    string
		upstreamTaskID       string
		expectedPollingPath  string
		upstreamResponse     string
		implementation       dto.LinkImplementationRef
		expectedActualTokens int
		expectedActualQuota  int
		expectedTier         string
		expectedReason       string
		expectedLogCount     int
	}{
		{
			name:                "official",
			channelID:           2601,
			modelName:           "seedance-byteplus",
			profile:             dto.VideoUpstreamProfileOfficial,
			preConsumeTokens:    520000,
			expectedPreConsume:  1820000,
			upstreamTaskID:      "upstream-official",
			expectedPollingPath: "/api/v3/contents/generations/tasks/upstream-official",
			upstreamResponse:    `{"id":"upstream-official","status":"succeeded","content":{"video_url":"https://cdn.example/official.mp4"},"usage":{"completion_tokens":108900,"total_tokens":108900}}`,
			implementation: dto.LinkImplementationRef{
				ID: model.LinkImplementationBytePlusSeedanceArk, Version: model.LinkImplementationVersionV1,
			},
			expectedActualTokens: actualTokens,
			expectedActualQuota:  actualQuota,
			expectedTier:         "480p720p",
			expectedReason:       "tokens=108900",
			expectedLogCount:     1,
		},
		{
			name:                "moxing_oversea_third_party_relay",
			channelID:           2801,
			modelName:           model.VideoSKUSeedance20Oversea,
			profile:             dto.VideoUpstreamProfileThirdPartyRelay,
			preConsumeTokens:    324000,
			expectedPreConsume:  1134000,
			queryPathTemplate:   "/v1/media/tasks/{task_id}",
			upstreamTaskID:      "upstream-moxing-relay",
			expectedPollingPath: "/v1/media/tasks/upstream-moxing-relay",
			upstreamResponse:    `{"task_id":"upstream-moxing-relay","status":"succeeded","result":{"url":"https://cdn.example/moxing-relay.mp4"},"usage":null}`,
			implementation: dto.LinkImplementationRef{
				ID: model.LinkImplementationMoxingSeedanceMedia, Version: model.LinkImplementationVersionV2,
			},
			expectedActualTokens: 0,
			expectedActualQuota:  1134000,
			expectedTier:         "480p720p",
			expectedReason:       "保持预扣额度",
			expectedLogCount:     0,
		},
		{
			name:                "tokensave_third_party_relay",
			channelID:           2701,
			modelName:           model.VideoSKUDoubaoSeedance20260128,
			profile:             dto.VideoUpstreamProfileThirdPartyRelay,
			preConsumeTokens:    1,
			expectedPreConsume:  365500,
			queryPathTemplate:   "/v1/media/tasks/{task_id}",
			upstreamTaskID:      "upstream-relay",
			expectedPollingPath: "/v1/media/tasks/upstream-relay",
			upstreamResponse:    `{"data":{"task_id":"upstream-relay","status":"succeeded","result":"{\"url\":\"https://cdn.example/relay.mp4\",\"duration_seconds\":5}","usage":"provider-defined"}}`,
			implementation: dto.LinkImplementationRef{
				ID: model.LinkImplementationTokenSaveSeedance, Version: model.LinkImplementationVersionV2,
			},
			expectedActualTokens: 0,
			expectedActualQuota:  365500,
			expectedTier:         "720p_text_or_reference",
			expectedReason:       "保持预扣额度",
			expectedLogCount:     0,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resetVideoE2ETables(t)

			requests := make(chan videoPollingRequest, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests <- videoPollingRequest{
					Method:        request.Method,
					Path:          request.URL.EscapedPath(),
					Authorization: request.Header.Get("Authorization"),
				}
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(writer, test.upstreamResponse)
			}))
			defer upstream.Close()

			const frozenKey = "sk-frozen-video-e2e"
			baseURL := upstream.URL
			channel := &model.Channel{
				Id:      test.channelID,
				Type:    constant.ChannelTypeDoubaoVideo,
				Name:    "seedance-e2e-" + test.name,
				Key:     "sk-current-channel-key",
				Status:  common.ChannelStatusEnabled,
				BaseURL: &baseURL,
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				VideoUpstreamProfile:           test.profile,
				VideoUpstreamQueryPathTemplate: test.queryPathTemplate,
				DisableTaskPollingSleep:        true,
				LinkImplementation:             test.implementation,
			})
			require.NoError(t, model.DB.Create(channel).Error)

			userID := 9600 + index
			require.NoError(t, model.DB.Create(&model.User{
				Id:       userID,
				Username: "seedance_e2e_" + test.name,
				Group:    "default",
				Quota:    initialQuota - test.expectedPreConsume,
				Status:   common.UserStatusEnabled,
			}).Error)

			priceContext, _ := gin.CreateTestContext(httptest.NewRecorder())
			priceContext.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", nil)
			priceContext.Set("task_request", relaycommon.TaskSubmitReq{
				Model:   test.modelName,
				Prompt:  "seedance e2e",
				Seconds: "5",
				Metadata: map[string]any{
					"resolution": "720p",
				},
			})
			priceInfo := &relaycommon.RelayInfo{
				OriginModelName: test.modelName,
				UserId:          userID,
				UserGroup:       "default",
				UsingGroup:      "default",
			}
			priceData, err := helper.ModelPriceHelperTaskTiered(priceContext, priceInfo, &doubao.TaskAdaptor{})
			require.NoError(t, err)
			assert.Equal(t, test.preConsumeTokens, priceInfo.TieredBillingSnapshot.EstimatedCompletionTokens)
			assert.Equal(t, test.expectedTier, priceInfo.TieredBillingSnapshot.EstimatedTier)
			assert.Equal(t, test.expectedPreConsume, priceData.Quota)

			task := &model.Task{
				TaskID:     "task-seedance-e2e-" + test.name,
				Platform:   constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)),
				UserId:     userID,
				Group:      "default",
				ChannelId:  test.channelID,
				Quota:      priceData.Quota,
				Action:     constant.TaskActionGenerate,
				Status:     model.TaskStatusInProgress,
				Progress:   "50%",
				SubmitTime: time.Now().Unix(),
				CreatedAt:  time.Now().Unix(),
				UpdatedAt:  time.Now().Unix(),
				Properties: model.Properties{
					OriginModelName:   test.modelName,
					UpstreamModelName: test.modelName,
				},
				PrivateData: model.TaskPrivateData{
					Key:                            frozenKey,
					UpstreamTaskID:                 test.upstreamTaskID,
					VideoUpstreamProfile:           test.profile,
					VideoUpstreamQueryBaseURL:      upstream.URL,
					VideoUpstreamQueryPathTemplate: test.queryPathTemplate,
					SouthboundAdapterVersion:       relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeDoubaoVideo, test.profile),
					LinkImplementationID:           test.implementation.ID,
					LinkImplementationVersion:      test.implementation.Version,
					BillingSource:                  service.BillingSourceWallet,
					BillingContext: &model.TaskBillingContext{
						GroupRatio:      1,
						OriginModelName: test.modelName,
					},
				},
			}
			model.AttachAsyncTaskBilling(&task.PrivateData, priceInfo, priceData.Quota)
			require.NoError(t, task.Insert())

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			require.NoError(t, service.UpdateVideoTasks(ctx, task.Platform, map[int][]string{
				test.channelID: {test.upstreamTaskID},
			}, map[string]*model.Task{
				test.upstreamTaskID: task,
			}))

			select {
			case request := <-requests:
				assert.Equal(t, http.MethodGet, request.Method)
				assert.Equal(t, test.expectedPollingPath, request.Path)
				assert.Equal(t, "Bearer "+frozenKey, request.Authorization)
			case <-time.After(time.Second):
				t.Fatal("mock upstream did not receive a polling request")
			}

			var settled model.Task
			require.NoError(t, model.DB.First(&settled, task.ID).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), settled.Status)
			assert.Equal(t, "100%", settled.Progress)
			assert.Equal(t, test.expectedActualQuota, settled.Quota)
			assert.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
			require.NotNil(t, settled.PrivateData.AsyncBilling)
			assert.Equal(t, test.expectedActualTokens, settled.PrivateData.AsyncBilling.ActualTokens)
			assert.Equal(t, test.preConsumeTokens, settled.PrivateData.AsyncBilling.EstimatedTokens)
			require.NotNil(t, settled.PrivateData.AsyncBilling.TargetQuota)
			assert.Equal(t, test.expectedActualQuota, *settled.PrivateData.AsyncBilling.TargetQuota)
			assert.Contains(t, settled.PrivateData.AsyncBilling.Reason, test.expectedReason)

			var storedResponse struct {
				Content struct {
					VideoURL string `json:"video_url"`
				} `json:"content"`
				Usage struct {
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}
			require.NoError(t, common.Unmarshal(settled.Data, &storedResponse))
			assert.Equal(t, test.expectedActualTokens, storedResponse.Usage.CompletionTokens)
			assert.Equal(t, test.expectedActualTokens, storedResponse.Usage.TotalTokens)
			assert.Equal(t, storedResponse.Content.VideoURL, settled.PrivateData.ResultURL)
			assert.NotEmpty(t, settled.PrivateData.ResultURL)

			var user model.User
			require.NoError(t, model.DB.Select("quota").First(&user, userID).Error)
			assert.Equal(t, initialQuota-test.expectedActualQuota, user.Quota)

			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ?", userID).Order("id").Find(&logs).Error)
			require.Len(t, logs, test.expectedLogCount)
			if test.expectedLogCount == 0 {
				return
			}
			refund := logs[0]
			assert.Equal(t, model.LogTypeRefund, refund.Type)
			assert.Equal(t, test.expectedPreConsume-actualQuota, refund.Quota)
			assert.Equal(t, test.expectedActualTokens, refund.CompletionTokens)
			assert.Equal(t, test.modelName, refund.ModelName)
			assert.Equal(t, test.channelID, refund.ChannelId)

			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(refund.Other, &other))
			assert.Equal(t, task.TaskID, other["task_id"])
			assert.Equal(t, float64(test.expectedPreConsume), other["pre_consumed_quota"])
			assert.Equal(t, float64(test.expectedActualQuota), other["actual_quota"])
		})
	}
}

func loadVideoE2EBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	modes := map[string]string{
		"seedance-byteplus":                  "tiered_expr",
		model.VideoSKUSeedance20Oversea:      "tiered_expr",
		model.VideoSKUDoubaoSeedance20260128: "tiered_expr",
	}
	expressions := map[string]string{
		"seedance-byteplus":                  seedanceSixTierExpression,
		model.VideoSKUSeedance20Oversea:      moxingOverseaTokenExpression,
		model.VideoSKUDoubaoSeedance20260128: tokenSavePerSecondExpression,
	}
	modeJSON, err := common.Marshal(modes)
	require.NoError(t, err)
	expressionJSON, err := common.Marshal(expressions)
	require.NoError(t, err)
	preConsumeJSON, err := common.Marshal(map[string]int{
		"seedance-byteplus":                  520000,
		model.VideoSKUSeedance20Oversea:      324000,
		model.VideoSKUDoubaoSeedance20260128: 1,
	})
	require.NoError(t, err)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":           string(modeJSON),
		"billing_setting.billing_expr":           string(expressionJSON),
		"task_billing_setting.preconsume_tokens": string(preConsumeJSON),
		"group_ratio_setting.group_ratio":        `{"default":1}`,
	}))
}

func resetVideoE2ETables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"tasks", "users", "logs", "channels"} {
		require.NoError(t, model.DB.Exec("DELETE FROM "+table).Error)
	}
}
