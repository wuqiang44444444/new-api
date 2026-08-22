package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictImageAPITypesDisablePassThrough(t *testing.T) {
	assert.True(t, isStrictImageAPIType(constant.APITypeAsyncImage))
	assert.False(t, isStrictImageAPIType(constant.APITypeOpenAI))
}

func TestStrictImageAPIRejectsParamOverrideBeforeProviderRequest(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAsyncImage)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, constant.FunCloudImageProviderModelNanoBanana2)
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, "{}")
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"resolution": "4K"})
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
		ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2,
	})

	request := &dto.ImageRequest{
		Model:  constant.FunCloudImageProviderModelNanoBanana2,
		Prompt: "test prompt",
	}
	apiErr := ImageHelper(c, &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: request.Model,
		Request:         request,
	})

	require.Error(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelParamOverrideInvalid, apiErr.GetErrorCode())
	assert.True(t, types.IsSkipRetryError(apiErr))
}
