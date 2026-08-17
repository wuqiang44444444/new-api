package seedance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceAdaptorRequiresModelArkV3Contract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(`{"model":"seedance-model","prompt":"hello"}`))
	context.Request.Header.Set("Content-Type", "application/json")

	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_contract", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestSeedanceAdaptorAcceptsModelArkV3Contract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"seedance-model","prompt":"hello"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   &dto.ModelArkVideoCreateRequest{Model: "seedance-model"},
	})
	context.Set(string(constant.ContextKeyTaskPromptValidated), true)

	adaptor := &TaskAdaptor{protocol: kitdto.VideoUpstreamProtocolModelArkV3Volcengine}
	taskErr := adaptor.ValidateRequestAndSetAction(context, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.Nil(t, taskErr)
}

func TestSeedanceAdaptorRejectsUnregisteredVideoProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{}`))
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   &dto.ModelArkVideoCreateRequest{Model: "seedance-model"},
	})

	taskErr := (&TaskAdaptor{protocol: "unregistered_video_protocol"}).ValidateRequestAndSetAction(
		context,
		&relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}},
	)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_protocol", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestSeedanceAdaptorRecordsModelArkGenerationMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		content  []dto.ModelArkVideoContent
		expected string
	}{
		{
			name: "text to video",
			content: []dto.ModelArkVideoContent{
				{Type: "text", Text: common.GetPointer("two puppies eating")},
			},
			expected: constant.TaskActionTextGenerate,
		},
		{
			name: "image to video",
			content: []dto.ModelArkVideoContent{
				{Type: "text", Text: common.GetPointer("move")},
				{Type: "image_url", Role: common.GetPointer("first_frame"), ImageURL: &dto.VideoMediaURL{URL: "https://example.com/first.png"}},
			},
			expected: constant.TaskActionGenerate,
		},
		{
			name: "first and last frame to video",
			content: []dto.ModelArkVideoContent{
				{Type: "image_url", Role: common.GetPointer("first_frame"), ImageURL: &dto.VideoMediaURL{URL: "https://example.com/first.png"}},
				{Type: "image_url", Role: common.GetPointer("last_frame"), ImageURL: &dto.VideoMediaURL{URL: "https://example.com/last.png"}},
			},
			expected: constant.TaskActionFirstTailGenerate,
		},
		{
			name: "reference image to video",
			content: []dto.ModelArkVideoContent{
				{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "https://example.com/reference.png"}},
			},
			expected: constant.TaskActionReferenceGenerate,
		},
		{
			name: "video input remains generic generate",
			content: []dto.ModelArkVideoContent{
				{Type: "text", Text: common.GetPointer("remix")},
				{Type: "video_url", VideoURL: &dto.VideoMediaURL{URL: "https://example.com/source.mp4"}},
			},
			expected: constant.TaskActionGenerate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{}`))
			context.Request.Header.Set("Content-Type", "application/json")
			relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
				ContractID: dto.VideoContractModelArkV3,
				ModelArk: &dto.ModelArkVideoCreateRequest{
					Model: "seedance-model", Content: test.content,
				},
			})
			context.Set(string(constant.ContextKeyTaskPromptValidated), true)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

			taskErr := (&TaskAdaptor{protocol: kitdto.VideoUpstreamProtocolModelArkV3Volcengine}).ValidateRequestAndSetAction(context, info)

			require.Nil(t, taskErr)
			assert.Equal(t, test.expected, info.Action)
		})
	}
}

func TestSeedanceTaskParserKeepsProviderBillingEvidenceOutOfPublicJSON(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"provider-task","status":"succeeded","content":{"video_url":"https://video.example.com/result.mp4"},
		"usage":{"completion_tokens":40594,"total_tokens":40594},
		"_provider_billing_evidence":{"provider":"funcloud","token_source":"completionTokens","reported_tokens":40594,"raw_consumption":"0.232731","consumption_unit":"pointConsume","provider_model":"seedance-2-fast","resolution":"720p"}
	}`))
	require.NoError(t, err)
	assert.True(t, result.UsageReported)
	assert.True(t, result.CompletionTokensReported)
	require.NotNil(t, result.ProviderBillingEvidence)
	assert.Equal(t, "completionTokens", result.ProviderBillingEvidence.TokenSource)
	assert.Equal(t, 40594, result.ProviderBillingEvidence.ReportedTokens)
	assert.Equal(t, "0.232731", result.ProviderBillingEvidence.RawConsumption)
	publicJSON, err := common.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(publicJSON), "provider_billing_evidence")
	assert.NotContains(t, string(publicJSON), "0.232731")
}

func TestSeedanceTaskParserPreservesReportedZeroUsage(t *testing.T) {
	result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{
		"id":"provider-task","status":"succeeded","content":{"video_url":"https://video.example.com/result.mp4"},
		"usage":{"completion_tokens":0,"total_tokens":0}
	}`))

	require.NoError(t, err)
	assert.True(t, result.UsageReported)
	assert.True(t, result.CompletionTokensReported)
	assert.Zero(t, result.CompletionTokens)
	assert.Zero(t, result.TotalTokens)
}

func TestMoxingFastTerminalUsageFlowsThroughNormalizationAndTaskParsing(t *testing.T) {
	normalized, err := normalizeVideoTaskResponse(
		kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk,
		relaycommon.VideoSouthboundAdapterVersion{
			ChannelType: constant.ChannelTypeSeedanceLink,
			Profile:     kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk,
			Revision:    relaycommon.VideoAdapterRevisionV1,
		},
		[]byte(`{"object":"media.task","task_id":"moxing-fast-1","status":"succeeded","model":"doubao-seedance-2-0-fast-260128","result":{"type":"video","primary_url":"https://video.example.com/result.mp4"},"usage":{"prompt_tokens":0,"completion_tokens":40594,"total_tokens":40594}}`),
		"moxing-fast-1",
		"https://www.moxing.pro",
		nil,
	)
	require.NoError(t, err)

	result, err := (&TaskAdaptor{}).ParseTaskResult(normalized)
	require.NoError(t, err)
	assert.True(t, result.UsageReported)
	assert.True(t, result.CompletionTokensReported)
	assert.Equal(t, 40594, result.CompletionTokens)
	assert.Equal(t, 40594, result.TotalTokens)
	assert.Equal(t, "usage.completion_tokens", result.UsageSource)
	assert.Equal(t, map[string]int{
		"usage.completion_tokens": 40594,
		"usage.prompt_tokens":     0,
		"usage.total_tokens":      40594,
	}, result.UsageEvidence)
}

func TestTokenSaveRejectsUnsupportedMediaDuringRequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		content dto.ModelArkVideoContent
	}{
		{
			name: "video",
			content: dto.ModelArkVideoContent{
				Type: "video_url", Role: common.GetPointer("reference_video"),
				VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"},
			},
		},
		{
			name: "audio",
			content: dto.ModelArkVideoContent{
				Type: "audio_url", Role: common.GetPointer("reference_audio"),
				AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
			relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
				ContractID: dto.VideoContractModelArkV3,
				ModelArk: &dto.ModelArkVideoCreateRequest{
					Model: modelSeedance20, Content: []dto.ModelArkVideoContent{test.content},
					Duration: common.GetPointer(5), Resolution: common.GetPointer("720p"),
				},
			})
			context.Set(string(constant.ContextKeyTaskPromptValidated), true)

			adaptor := &TaskAdaptor{protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1}
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: modelSeedance20},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
			taskErr := adaptor.ValidateMappedRequest(context, info)

			require.NotNil(t, taskErr)
			assert.Equal(t, "invalid_video_parameter", taskErr.Code)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
		})
	}
}

func TestMoxingRejectsUnsupportedSeedDuringMappedValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model: modelSeedance20,
			Content: []dto.ModelArkVideoContent{{
				Type: "text", Text: common.GetPointer("generate"),
			}},
			Duration: common.GetPointer(5), Resolution: common.GetPointer("720p"),
			Seed: common.GetPointer(24),
		},
	})
	adaptor := &TaskAdaptor{protocol: kitdto.VideoUpstreamProtocolMoxingMediaTaskV1}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelSeedance20}}

	taskErr := adaptor.ValidateMappedRequest(context, info)

	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_parameter", taskErr.Code)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}

func TestFunCloudRequiresTieredBillingBeforeProviderSubmission(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`,
	}))

	newContext := func() *gin.Context {
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
		relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
			ContractID: dto.VideoContractModelArkV3,
			ModelArk: &dto.ModelArkVideoCreateRequest{
				Model: modelFunCloud20Fast, Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
				Duration: common.GetPointer(5), Resolution: common.GetPointer("720p"),
			},
		})
		context.Set(string(constant.ContextKeyTaskPromptValidated), true)
		return context
	}
	info := func() *relaycommon.RelayInfo {
		return &relaycommon.RelayInfo{
			OriginModelName: "seedance-2-fast-funcloud",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: modelFunCloud20Fast,
			},
			TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		}
	}
	adaptor := &TaskAdaptor{
		protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance,
		profile:  kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedance,
		baseURL:  "https://funcloud.example.com",
	}
	taskErr := adaptor.ValidateRequestAndSetAction(newContext(), info())
	require.NotNil(t, taskErr)
	assert.Equal(t, "model_price_error", taskErr.Code)

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"seedance-2-fast-funcloud":"tiered_expr"}`,
	}))
	require.Nil(t, adaptor.ValidateRequestAndSetAction(newContext(), info()))
}

func TestFunCloudMappedValidationBindsExactProviderPath(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model: modelFunCloud20Mini, Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
			Duration: common.GetPointer(5), Resolution: common.GetPointer("720p"),
		},
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "seedance-2-mini-funcloud",
			ChannelBaseUrl:    "https://funcloud.example.com",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProtocol: kitdto.VideoUpstreamProtocolFunCloudSeedance,
			},
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	assert.Empty(t, adaptor.createPath, "customer model must not be guessed as a Provider path")
	info.UpstreamModelName = modelFunCloud20Mini
	require.Nil(t, adaptor.ValidateMappedRequest(context, info))
	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://funcloud.example.com/api/v2/open/aigc/seedance2-0-mini", requestURL)
	assert.Equal(t, "/api/v2/open/aigc/seedance2-0-mini", info.ChannelOtherSettings.VideoUpstreamCreatePath)
	assert.Equal(t, "/api/v2/open/aigc/{task_id}", info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate)
}
