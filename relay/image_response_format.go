package relay

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// PrepareImageResponseFormat checks delivery dependencies before billing. It
// leaves absent formats and native SSE untouched. Model mapping is evaluated on
// a copy through the native mapper, never a second routing/capability registry.
func PrepareImageResponseFormat(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	request, ok := info.Request.(*dto.ImageRequest)
	if !ok || request.ResponseFormat == "" {
		return nil
	}
	format := strings.TrimSpace(request.ResponseFormat)
	if format != "" && format != "url" && format != "b64_json" {
		return types.NewErrorWithStatusCode(errors.New("response_format must be url or b64_json"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if format != "url" || service.ImageAsyncExecutionRequested(c) || (request.Stream != nil && *request.Stream) {
		return nil
	}
	preview := *info
	// GenRelayInfo has no ChannelMeta yet. Read the actual selected channel
	// from the native context on every attempt, without mutating billing state.
	preview.InitChannelMeta(c)
	if err := helper.ModelMappedHelper(c, &preview, nil); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if !nativeImageBase64Only(&preview) && preview.ApiType != constant.APITypeGemini && preview.ApiType != constant.APITypeVertexAi {
		return nil
	}
	ctx, err := service.WithImageObjectStore(c.Request.Context())
	if err == nil {
		err = service.CheckImageObjectStoreReady(ctx)
	}
	if err != nil {
		return types.NewErrorWithStatusCode(errors.New("response_format=url requires available platform object storage"), types.ErrorCodeInvalidRequest, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	c.Request = c.Request.WithContext(ctx)
	return nil
}

// Native GPT Image has no upstream response_format parameter. This only
// describes that provider wire protocol; it does not select a channel or Link
// execution mode, or constrain unknown compatible image models.
func nativeImageBase64Only(info *relaycommon.RelayInfo) bool {
	switch info.ApiType {
	case constant.APITypeOpenAI, constant.APITypeNewAPI, constant.APITypeMoonshot, constant.APITypeAdvancedCustom:
	default:
		return false
	}
	model := info.UpstreamModelName
	// Azure deployments may be opaque; use the same known customer profile as
	// the existing native metadata projection, without changing model mapping.
	if info.ChannelType == constant.ChannelTypeAzure && !common.IsImageGenerationModel(model) {
		model = info.OriginModelName
	}
	return strings.HasPrefix(model, "gpt-image-") || model == "chatgpt-image-latest"
}

// Keep the client format on info.Request. These adapters have fixed provider
// formats; the standard image response delivery owns any necessary conversion.
func prepareImageProviderFormat(info *relaycommon.RelayInfo, request *dto.ImageRequest) {
	if strings.TrimSpace(request.ResponseFormat) == "" {
		return
	}
	switch info.ApiType {
	case constant.APITypeGemini, constant.APITypeVertexAi:
		request.ResponseFormat = "b64_json"
	case constant.APITypeAsyncImage, constant.APITypeAli:
		request.ResponseFormat = "url"
	}
}
