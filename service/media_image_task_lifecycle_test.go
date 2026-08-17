package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	mediaimageprotocol "github.com/QuantumNous/new-api/relay/mediaimage"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mediaImageRoundTripper func(*http.Request) (*http.Response, error)

const testMediaImageQueryPath = "/v1/media/tasks/{task_id}"

func (f mediaImageRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestPersistMediaImageTaskTransfersBillingWithoutPersistingPrompt(t *testing.T) {
	truncate(t)
	seedUser(t, 801, 5_000_000)
	token := model.Token{
		UserId:      801,
		Key:         "media-image-persist-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 5_000_000,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"prompt":"must-not-persist"}`))
	info := &relaycommon.RelayInfo{
		UserId:                801,
		TokenId:               token.Id,
		TokenKey:              token.Key,
		UsingGroup:            "default",
		OriginModelName:       "seedream-5-0-260128",
		FinalPreConsumedQuota: 1_000_000,
		PriceData: types.PriceData{
			ModelPrice:     1,
			UsePrice:       true,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			ClientProtocol: model.TaskClientProtocolOpenAIImages,
			Action:         constant.TaskActionImageGeneration,
			PublicTaskID:   "task_public_image_persist",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:      91,
			ChannelBaseUrl: "https://provider.example",
			ApiKey:         "provider-secret",
		},
	}
	info.PriceData.AddOtherRatio("n", 2)
	attempt, err := model.CreatePreparedTaskAttempt(model.TaskCreateAttemptParams{
		PublicTaskID:   info.TaskRelayInfo.PublicTaskID,
		UserID:         info.UserId,
		TokenID:        info.TokenId,
		ClientProtocol: info.TaskRelayInfo.ClientProtocol,
		RequestHash:    "media-image-persistence-test",
	})
	require.NoError(t, err)
	_, err = model.HoldTaskCreateAttempt(model.TaskAttemptHoldParams{
		AttemptID:     attempt.ID,
		FundingSource: BillingSourceWallet,
		Quota:         info.FinalPreConsumedQuota,
	})
	require.NoError(t, err)
	common.SetContextKey(c, constant.ContextKeyTaskCreateAttemptID, int(attempt.ID))

	task, err := PersistMediaImageTask(c, info, MediaImageTaskCreateSpec{
		UpstreamTaskID:      "upstream-task-1",
		Protocol:            mediaimageprotocol.ProtocolMediaImageTaskV1,
		QueryBaseURL:        "https://provider.example",
		QueryPathTemplate:   testMediaImageQueryPath,
		ResponseFormat:      "url",
		RequestedImageCount: 2,
	})
	require.NoError(t, err)
	assert.True(t, info.BillingTransferredToTask)
	require.NotNil(t, info.PersistedImageTask)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), task.Status)
	assert.Equal(t, 1_000_000, task.Quota)

	privateJSON, err := common.Marshal(task.PrivateData)
	require.NoError(t, err)
	assert.NotContains(t, string(privateJSON), "must-not-persist")
	assert.NotContains(t, string(privateJSON), "reference_images")
	assert.NotContains(t, string(privateJSON), "video_upstream")

	stored, exists, err := model.GetByOnlyTaskId(task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "upstream-task-1", stored.PrivateData.UpstreamTaskID)
	require.NotNil(t, stored.PrivateData.MediaImage)
	assert.Equal(t, mediaimageprotocol.ProtocolMediaImageTaskV1, stored.PrivateData.MediaImage.Protocol)
	assert.Equal(t, testMediaImageQueryPath, stored.PrivateData.MediaImage.QueryPathTemplate)
	assert.Equal(t, model.TaskBillingStatePending, stored.BillingState)
}

func TestPollMediaImageTaskOncePersistsAllURLsAndSettlesActualCountOnce(t *testing.T) {
	truncate(t)
	seedUser(t, 802, 2_000_000)

	task := &model.Task{
		TaskID:         "task_public_image",
		CreatedAt:      100,
		UpdatedAt:      100,
		SubmitTime:     100,
		UserId:         802,
		Group:          "default",
		ChannelId:      92,
		Quota:          1_000_000,
		Action:         constant.TaskActionImageGeneration,
		Status:         model.TaskStatusQueued,
		Progress:       "0%",
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
		Properties:     model.Properties{OriginModelName: "seedream-5-0-260128"},
		PrivateData: model.TaskPrivateData{
			Key:            "provider-secret",
			UpstreamTaskID: "provider-task-2",
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 1, GroupRatio: 1, OriginModelName: "seedream-5-0-260128",
				OtherRatios: map[string]float64{"n": 2},
			},
			MediaImage: &model.TaskMediaImagePrivateData{
				Protocol:            mediaimageprotocol.ProtocolMediaImageTaskV1,
				QueryBaseURL:        "https://provider.example",
				QueryPathTemplate:   testMediaImageQueryPath,
				RequestedImageCount: 2,
				ResponseFormat:      "url",
				UsePrice:            true,
			},
			AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
		BillingState: model.TaskBillingStatePending,
	}
	require.NoError(t, task.Insert())

	oldClient := httpClient
	calls := 0
	httpClient = &http.Client{Transport: mediaImageRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		assert.Equal(t, "/v1/media/tasks/provider-task-2", request.URL.Path)
		assert.Equal(t, "Bearer provider-secret", request.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"X-Request-Id": []string{"poll-request-2"}},
			Body: io.NopCloser(strings.NewReader(
				`{"data":{"status":"succeeded","result":{"primary_url":"https://cdn.example/one.png","urls":["https://cdn.example/one.png"]}}}`,
			)),
			Request: request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = oldClient })

	completed, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), completed.Status)
	require.NotNil(t, completed.PrivateData.MediaImage)
	assert.Equal(t, []string{"https://cdn.example/one.png"}, completed.PrivateData.MediaImage.ResultURLs)
	assert.Equal(t, "poll-request-2", completed.PrivateData.MediaImage.LastPollRequestID)
	assert.Equal(t, int(common.QuotaPerUnit), completed.Quota)
	assert.Equal(t, 1, calls)

	again, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), again.Status)
	assert.Equal(t, 1, calls, "terminal replay must not query or settle again")
}

func TestSettleMediaImageTaskAppliesFrozenCustomerContractToActualCount(t *testing.T) {
	truncate(t)
	seedUser(t, 804, 2_000_000)

	task := &model.Task{
		TaskID: "task_contract_image_count", UserId: 804, Group: "contract-group", ChannelId: 94,
		Quota: 696_000, Status: model.TaskStatusSuccess, Platform: constant.TaskPlatformMediaImage,
		Properties: model.Properties{OriginModelName: "contract-model"},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 1, GroupRatio: 0.87, OriginModelName: "contract-model",
				OtherRatios: map[string]float64{"n": 2},
				ContractFact: &types.ContractBillingFact{
					UserId: 804, ContractVersion: 2, PublicModel: "contract-model",
					RouteGroup: "contract-group", RatioUnits: 80_000_000,
				},
			},
			MediaImage: &model.TaskMediaImagePrivateData{
				UsePrice: true, RequestedImageCount: 2, ResultURLs: []string{"https://cdn.example/one.png"},
			},
			AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
		BillingState: model.TaskBillingStatePending,
	}
	require.NoError(t, task.Insert())

	settleMediaImageTask(context.Background(), task)

	stored, exists, err := model.GetByOnlyTaskId(task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	// 1 image × 1 price × 500000 × 0.87 native group × 0.8 contract.
	assert.Equal(t, 348_000, stored.Quota)
	require.NotNil(t, stored.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 348_000, *stored.PrivateData.AsyncBilling.TargetQuota)
}

func TestSettleMediaImageTaskAppliesFrozenCustomerContractToTieredUsage(t *testing.T) {
	truncate(t)
	seedUser(t, 805, 2_000_000)
	expression := `tier("base", p * 2)`

	task := &model.Task{
		TaskID: "task_contract_image_tiered", UserId: 805, Group: "contract-group", ChannelId: 95,
		Quota: 100, Status: model.TaskStatusSuccess, Platform: constant.TaskPlatformMediaImage,
		Properties: model.Properties{OriginModelName: "contract-model"},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "contract-model",
				ContractFact: &types.ContractBillingFact{
					UserId: 805, ContractVersion: 3, PublicModel: "contract-model",
					RouteGroup: "contract-group", RatioUnits: 80_000_000,
				},
			},
			MediaImage: &model.TaskMediaImagePrivateData{Usage: &dto.Usage{PromptTokens: 100, TotalTokens: 100}},
			AsyncBilling: &model.TaskAsyncBillingContext{
				State: model.TaskBillingStatePending,
				TieredSnapshot: &billingexpr.BillingSnapshot{
					BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
					GroupRatio: 0.87, EstimatedQuotaAfterGroup: 87, QuotaPerUnit: common.QuotaPerUnit,
				},
			},
		},
		BillingState: model.TaskBillingStatePending,
	}
	require.NoError(t, task.Insert())

	settleMediaImageTask(context.Background(), task)

	stored, exists, err := model.GetByOnlyTaskId(task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	// Raw quota 100 × native group 0.87 × contract 0.8 = 69.6, rounded once.
	assert.Equal(t, 70, stored.Quota)
	require.NotNil(t, stored.PrivateData.AsyncBilling.TargetQuota)
	assert.Equal(t, 70, *stored.PrivateData.AsyncBilling.TargetQuota)
}

func TestPollMediaImageTaskOnceFailsClosedWhenProviderReturnsMoreThanRequested(t *testing.T) {
	truncate(t)
	seedUser(t, 803, 1_000_000)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:         "task_image_overdelivery",
		CreatedAt:      now,
		UpdatedAt:      now,
		SubmitTime:     now,
		UserId:         803,
		Group:          "default",
		ChannelId:      93,
		Quota:          1_000_000,
		Action:         constant.TaskActionImageGeneration,
		Status:         model.TaskStatusQueued,
		Progress:       "0%",
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
		Properties:     model.Properties{OriginModelName: "seedream-5-0-260128"},
		PrivateData: model.TaskPrivateData{
			Key:            "provider-secret",
			UpstreamTaskID: "provider-task-overdelivery",
			BillingContext: &model.TaskBillingContext{
				ModelPrice: 1, GroupRatio: 1, OriginModelName: "seedream-5-0-260128",
				OtherRatios: map[string]float64{"n": 1},
			},
			MediaImage: &model.TaskMediaImagePrivateData{
				Protocol:            mediaimageprotocol.ProtocolMediaImageTaskV1,
				QueryBaseURL:        "https://provider.example",
				QueryPathTemplate:   testMediaImageQueryPath,
				RequestedImageCount: 1,
				ResponseFormat:      "url",
				UsePrice:            true,
			},
			AsyncBilling: &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending},
		},
	}
	require.NoError(t, task.Insert())

	oldClient := httpClient
	calls := 0
	httpClient = &http.Client{Transport: mediaImageRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"status":"succeeded","result":{"urls":["https://cdn.example/one.png","https://cdn.example/two.png"]}}`,
			)),
			Request: request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = oldClient })

	completed, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusProviderContractFailure), completed.Status)
	assert.Equal(t, "provider_contract_failure", completed.FailReason)
	assert.Zero(t, completed.Quota)
	assert.Empty(t, completed.PrivateData.MediaImage.ResultURLs)
	require.NotNil(t, completed.PrivateData.AsyncBilling)
	assert.Equal(t, model.TaskBillingStateSettled, completed.PrivateData.AsyncBilling.State)
	assert.Equal(t, 2_000_000, getUserQuota(t, 803))
	var exposure model.ProviderCostExposure
	require.NoError(t, model.DB.First(&exposure,
		"source_kind = ? AND source_id = ? AND reason = ?",
		model.ProviderCostExposureSourceTask,
		task.TaskID,
		"provider_contract_failure",
	).Error)
	assert.Equal(t, 1_000_000, exposure.CustomerQuotaReleased)

	again, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusProviderContractFailure), again.Status)
	assert.Equal(t, 1, calls, "terminal replay must not query or refund again")
}

func TestSweepTimedOutTasksProtectsMediaImageMinimumLifecycle(t *testing.T) {
	truncate(t)
	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })

	now := time.Now().Unix()
	tasks := []*model.Task{
		{
			TaskID: "media_image_still_protected", SubmitTime: now - 10*60,
			Status: model.TaskStatusQueued, Progress: "0%",
			Platform: constant.TaskPlatformMediaImage, ClientProtocol: model.TaskClientProtocolOpenAIImages,
		},
		{
			TaskID: "media_image_past_floor", SubmitTime: now - 31*60,
			Status: model.TaskStatusQueued, Progress: "0%",
			Platform: constant.TaskPlatformMediaImage, ClientProtocol: model.TaskClientProtocolOpenAIImages,
		},
		{
			TaskID: "other_task_uses_configured_timeout", SubmitTime: now - 10*60,
			Status: model.TaskStatusQueued, Progress: "0%",
			Platform: constant.TaskPlatform("test_video"),
		},
	}
	for _, task := range tasks {
		require.NoError(t, task.Insert())
	}

	sweepTimedOutTasks(context.Background())

	protected, exists, err := model.GetByOnlyTaskId("media_image_still_protected")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), protected.Status)

	expiredImage, exists, err := model.GetByOnlyTaskId("media_image_past_floor")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), expiredImage.Status)
	assert.Contains(t, expiredImage.FailReason, "30分钟")

	expiredOther, exists, err := model.GetByOnlyTaskId("other_task_uses_configured_timeout")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), expiredOther.Status)
	assert.Contains(t, expiredOther.FailReason, "1分钟")
}

func TestValidateMediaImageUsageRejectsBillingOverflow(t *testing.T) {
	usage := &dto.Usage{TotalTokens: -1}
	require.Error(t, validateMediaImageUsage(usage))
}

func TestNormalizeMediaImageUsageMapsResponsesTokenFields(t *testing.T) {
	normalized, err := normalizeMediaImageUsage(&dto.Usage{
		InputTokens:  11,
		OutputTokens: 7,
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 3,
			TextTokens:   5,
			ImageTokens:  6,
		},
		UsageSource:  "untrusted",
		BillingUsage: &dto.BillingUsage{Source: "untrusted"},
		Cost:         map[string]any{"secret": "opaque"},
	})
	require.NoError(t, err)
	require.NotNil(t, normalized)
	assert.Equal(t, 11, normalized.PromptTokens)
	assert.Equal(t, 7, normalized.CompletionTokens)
	assert.Equal(t, 18, normalized.TotalTokens)
	assert.Equal(t, 3, normalized.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 5, normalized.PromptTokensDetails.TextTokens)
	assert.Equal(t, 6, normalized.PromptTokensDetails.ImageTokens)
	assert.Empty(t, normalized.UsageSource)
	assert.Nil(t, normalized.BillingUsage)
	assert.Nil(t, normalized.Cost)
}

func TestDecodeMediaImageTaskUsageSupportsGeminiComponentUsage(t *testing.T) {
	usage, err := decodeMediaImageTaskUsage(json.RawMessage(`{
		"promptTokenCount":12,
		"promptTokensDetails":[{"modality":"TEXT","tokenCount":12}],
		"candidatesTokenCount":1120,
		"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":1120}],
		"totalTokenCount":1132,
		"trafficType":"ON_DEMAND"
	}`))

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 12, usage.PromptTokens)
	assert.Equal(t, 1120, usage.CompletionTokens)
	assert.Equal(t, 1132, usage.TotalTokens)
	assert.Equal(t, 12, usage.PromptTokensDetails.TextTokens)
	assert.Zero(t, usage.PromptTokensDetails.ImageTokens)
	assert.Equal(t, 1120, usage.CompletionTokenDetails.ImageTokens)
	assert.Equal(t, dto.BillingUsageSemanticGemini, usage.UsageSemantic)
	assert.Nil(t, usage.BillingUsage)
}
