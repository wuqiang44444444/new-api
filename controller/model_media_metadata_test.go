package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieveGPTImage2ReturnsSameEndpointAndParameterMetadataAsList(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)
	channel := &model.Channel{
		Id: 801, Type: constant.ChannelTypeOpenAI, Key: "openai-key", Status: common.ChannelStatusEnabled,
		Name: "native-image", Group: "default", Models: "gpt-image-2",
	}
	require.NoError(t, channel.Insert())
	model.InvalidatePricingCache()
	model.GetPricing()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/models/gpt-image-2", nil)
	context.Params = gin.Params{{Key: "model", Value: "gpt-image-2"}}
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	RetrieveModel(context, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response dto.OpenAIModels
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Contains(t, response.SupportedEndpointTypes, constant.EndpointTypeImageGeneration)
	require.NotNil(t, response.API)
	require.NotNil(t, response.API.Image)
	assert.Equal(t, "/v1/images/generations", response.API.Image.Creation.Path)
	assert.NotEmpty(t, response.API.Image.Creation.Parameters)
}

func TestRetrieveCustomImageAliasReturnsStrictMappedParameters(t *testing.T) {
	withSelfUseModeEnabled(t)
	setupModelListControllerTestDB(t)
	baseURL := "https://images.example.com"
	channel := &model.Channel{
		Id: 802, Type: constant.ChannelTypeAsyncImage, Key: "image-key", Status: common.ChannelStatusEnabled,
		Name: "strict-image", Group: "default", Models: "customer-seedream-pro", BaseURL: &baseURL,
		ModelMapping: common.GetPointer(`{"customer-seedream-pro":"doubao-seedream-5-0-pro-260628"}`),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolMoxingImagesV1})
	require.NoError(t, channel.Insert())
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/models/customer-seedream-pro", nil)
	context.Params = gin.Params{{Key: "model", Value: "customer-seedream-pro"}}
	common.SetContextKey(context, constant.ContextKeyUserGroup, "default")
	RetrieveModel(context, constant.ChannelTypeOpenAI)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response dto.OpenAIModels
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Contains(t, response.SupportedEndpointTypes, constant.EndpointTypeImageGeneration)
	require.NotNil(t, response.API)
	require.NotNil(t, response.API.Image)
	for _, parameter := range response.API.Image.Creation.Parameters {
		assert.NotEqual(t, "watermark", parameter.Name)
		if parameter.Name == "size" {
			assert.Equal(t, []string{"2K"}, parameter.Enum)
		}
	}
}
