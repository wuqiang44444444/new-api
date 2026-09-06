package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/asyncimage"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageAsyncPreservesParametersAndSeparatesProviderDelivery(t *testing.T) {
	request := &dto.ImageRequest{}
	require.NoError(t, common.Unmarshal([]byte(`{"model":"nano-banana-2","prompt":"p","size":"1K","output_format":"png","response_format":"b64_json","stream":false,"user":"customer"}`), request))
	snapshot, err := common.DeepCopy(request)
	require.NoError(t, err)
	rebuilt, err := rebuildImageRequest(context.Background(), &model.TaskImageExecutionData{
		Parameters: snapshot, UpstreamModel: request.Model, ResponseFormat: "b64_json", N: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, request.OutputFormat, rebuilt.OutputFormat)
	assert.Equal(t, request.User, rebuilt.User)
	require.NotNil(t, rebuilt.Stream)
	assert.False(t, *rebuilt.Stream)
	// The stored northbound user field is preserved; this Provider profile does
	// not publish user, so the accepted southbound example leaves it unset.
	rebuilt.User = nil
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAsyncImage, UpstreamModelName: request.Model, ChannelOtherSettings: dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2}},
	}
	require.Nil(t, validateImageAsyncFamilyContract(nil, info, rebuilt))
	assert.Equal(t, "b64_json", rebuilt.ResponseFormat, "northbound response format must not be overwritten during validation")
	provider := *rebuilt
	provider.ResponseFormat = "url"
	converted, err := (&asyncimage.Adaptor{}).ConvertImageRequest(nil, info, provider)
	require.NoError(t, err)
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"outputFormat":"png"`)
	rebuilt.Quality = "unsupported"
	apiErr := validateImageAsyncFamilyContract(nil, info, rebuilt)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, "reject before 202")
}

func TestImageModelMappingDoesNotEraseValidationEvidence(t *testing.T) {
	c := &gin.Context{Request: &http.Request{Header: http.Header{}}}
	request := &dto.ImageRequest{}
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gemini-3.1-flash-image","prompt":"p","n":0,"private_option":true}`), request))
	n := uint(1)
	request.N = &n // native parser normalization
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeGemini, UpstreamModelName: request.Model}, RelayMode: relayconstant.RelayModeImagesGenerations}
	mapped, err := mapImageAsyncRequest(c, info, request)
	require.NoError(t, err)
	assert.True(t, mapped.NExplicitZero)
	assert.Contains(t, mapped.Extra, "private_option")
	apiErr := validateImageAsyncFamilyContract(c, info, mapped)
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
}

func TestGoogleImageRouteRejectsParserEvidenceBeforeProviderCall(t *testing.T) {
	for _, body := range []string{
		`{"model":"gemini-3.1-flash-image","prompt":"p","n":0}`,
		`{"model":"gemini-3.1-flash-image","prompt":"p","private_option":true}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.invalid")
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-3.1-flash-image")
		request := &dto.ImageRequest{}
		require.NoError(t, common.Unmarshal([]byte(body), request))
		n := uint(1)
		request.N = &n
		apiErr := ImageHelper(c, &relaycommon.RelayInfo{
			RelayMode: relayconstant.RelayModeImagesGenerations, OriginModelName: request.Model, Request: request,
		})
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	}
}
