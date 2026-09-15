package controller

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelTestImageRequestUsesModelCompatibleSize(t *testing.T) {
	tests := []struct {
		name  string
		model string
		size  string
	}{
		{name: "Gemini usage price", model: "gemini-3.1-flash-image-preview-usage", size: "1K"},
		{name: "Nano Banana 2 public SKU", model: "nano-banana-2", size: "1K"},
		{name: "Nano Banana 2 Lite", model: "nano-banana-2-lite", size: ""},
		{name: "Seedream 4.5", model: "doubao-seedream-4-5-251128", size: "2048x2048"},
		{name: "Seedream 5.0", model: "seedream-5-0-260128", size: "2K"},
		{name: "Moxing Seedream public SKU", model: "seedream-5-moxing", size: "2K"},
		{name: "Qihang Seedream public SKU", model: "seedream-5-qihang", size: "2K"},
		{name: "Seedream 5 Lite", model: "seedream-5.0-lite", size: "2K"},
		{name: "Seedream 5 Pro", model: "seedream-5.0-pro", size: "1K"},
		{name: "Default", model: "other-image-model", size: "1024x1024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := buildChannelTestImageRequest(tt.model)

			require.NotNil(t, request.N)
			assert.Equal(t, uint(1), *request.N)
			assert.Equal(t, tt.size, request.Size)
		})
	}
}

func TestNormalizeChannelTestEndpointUsesAsyncImageEndpoint(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAsyncImage}
	assert.Equal(t, string(constant.EndpointTypeImageGeneration), normalizeChannelTestEndpoint(channel, "nano-banana-2", ""))
}

func TestBuildChannelTestImageRequestForImageRelayUsesSelectedProtocolProfile(t *testing.T) {
	channel := &model.Channel{
		Type:   constant.ChannelTypeAsyncImage,
		Models: "lite-customer-model,pro-customer-model",
		ModelMapping: common.GetPointer(`{
			"lite-customer-model":"doubao-seedream-5-0-260128",
			"pro-customer-model":"doubao-seedream-5-0-pro-260628"
		}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1,
	})
	tests := []struct {
		name          string
		customerModel string
		wantSize      string
	}{
		{name: "lite", customerModel: "lite-customer-model", wantSize: constant.MoxingImageSeedream5LiteSize},
		{name: "pro", customerModel: "pro-customer-model", wantSize: constant.MoxingImageSeedream5ProSize},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := buildChannelTestImageRequestForChannel(channel, test.customerModel)

			require.NotNil(t, request.N)
			assert.Equal(t, uint(1), *request.N)
			assert.Equal(t, test.wantSize, request.Size)
		})
	}
}

// Exercise the same endpoint selection, mapping and conversion used by channel tests.
func TestBuildChannelTestImageRequestForGeminiUsesContractValidSize(t *testing.T) {
	for _, tc := range []struct {
		name, customerModel, providerModel string
		channelType                        int
	}{
		{"Gemini alias", "nano-banana-2", "gemini-3.1-flash-image", constant.ChannelTypeGemini},
		{"Gemini lite alias", "nano-banana-2-lite", "gemini-3.1-flash-image", constant.ChannelTypeGemini},
		{"Gemini direct model", "gemini-3.1-flash-image", "", constant.ChannelTypeGemini},
		{"Vertex alias", "nano-banana-2", "gemini-3.1-flash-image", constant.ChannelTypeVertexAi},
		{"Imagen alias", "nano-banana-2", "imagen-4.0-generate-001", constant.ChannelTypeGemini},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &model.Channel{Type: tc.channelType}
			providerModel := tc.customerModel
			if tc.providerModel != "" {
				mapping, err := common.Marshal(map[string]string{tc.customerModel: tc.providerModel})
				require.NoError(t, err)
				channel.ModelMapping = common.GetPointer(string(mapping))
				providerModel = tc.providerModel
			}
			endpoint := normalizeChannelTestEndpoint(channel, tc.customerModel, "")
			if tc.name == "Imagen alias" {
				// Imagen aliases use the explicitly selected image endpoint.
				endpoint = normalizeChannelTestEndpoint(channel, tc.customerModel, string(constant.EndpointTypeImageGeneration))
			}
			require.Equal(t, string(constant.EndpointTypeImageGeneration), endpoint)
			request, ok := buildTestRequest(tc.customerModel, endpoint, channel, false).(*dto.ImageRequest)
			require.True(t, ok)
			assert.Equal(t, "1024x1024", request.Size)
			require.NotNil(t, request.N)
			assert.Equal(t, uint(1), *request.N)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
			c.Set("model_mapping", channel.GetModelMapping())
			apiType, supported := common.ChannelType2APIType(channel.Type)
			require.True(t, supported)
			info := &relaycommon.RelayInfo{
				OriginModelName: tc.customerModel, Request: request, RelayMode: relayconstant.RelayModeImagesGenerations,
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType: apiType, UpstreamModelName: tc.customerModel, ChannelBaseUrl: "https://provider.invalid",
					ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey},
				},
			}
			require.NoError(t, helper.ModelMappedHelper(c, info, request))
			assert.Equal(t, providerModel, request.Model)
			assert.Equal(t, providerModel, info.UpstreamModelName)
			assert.Equal(t, tc.customerModel, info.GetBillingModelName(), "mapping must not change the customer's pricing identity")
			adaptor := relay.GetAdaptor(apiType)
			require.NotNil(t, adaptor)
			adaptor.Init(info)
			converted, err := adaptor.ConvertImageRequest(c, info, *request)
			require.NoError(t, err)
			requestURL, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			parsedURL, err := url.Parse(requestURL)
			require.NoError(t, err)
			if tc.name == "Imagen alias" {
				imagen, ok := converted.(dto.GeminiImageRequest)
				require.True(t, ok)
				assert.Equal(t, 1, imagen.Parameters.SampleCount)
				assert.Equal(t, "1:1", imagen.Parameters.AspectRatio)
				assert.Contains(t, parsedURL.Path, "/models/"+providerModel+":predict")
				return
			}
			geminiRequest, ok := converted.(*dto.GeminiChatRequest)
			require.True(t, ok)
			assert.Equal(t, []string{"TEXT", "IMAGE"}, geminiRequest.GenerationConfig.ResponseModalities)
			require.Len(t, geminiRequest.Contents, 1)
			require.Len(t, geminiRequest.Contents[0].Parts, 1)
			assert.Equal(t, request.Prompt, geminiRequest.Contents[0].Parts[0].Text)
			assert.Contains(t, parsedURL.Path, "/models/"+providerModel+":generateContent")
		})
	}
}
