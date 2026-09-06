package relay

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Standard Google image generation has no idempotent resend contract. A failed
// HTTP exchange cannot prove that the Provider did not receive the generation.
func imageRequestTransportError(info *relaycommon.RelayInfo, err error) *types.NewAPIError {
	apiErr := types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	if (info.ApiType == constant.APITypeGemini || info.ApiType == constant.APITypeVertexAi) &&
		gemini.SupportsGenerateContentImage(info.UpstreamModelName) && apiErr.GetErrorCode() == types.ErrorCodeDoRequestFailed {
		types.ErrOptionWithSkipRetry()(apiErr)
	}
	return apiErr
}
