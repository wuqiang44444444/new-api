package moxingimage

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

type imageUsage struct {
	OutputTokens *json.Number `json:"output_tokens,omitempty"`
	InputImages  *json.Number `json:"input_images,omitempty"`
}

// normalizeResult is the single response contract for synchronous and worker calls.
func normalizeResult(provider providerResponse, model string) ([]string, *dto.Usage, *types.NewAPIError) {
	if hasProviderError(provider.Error) {
		return nil, nil, providerApplicationError(provider.Error)
	}
	if name := strings.TrimSpace(provider.Model); name != "" && name != model {
		return nil, nil, upstreamError("image response model does not match the request")
	}
	if len(provider.Data) != 1 {
		return nil, nil, upstreamError("image provider result must contain exactly one image")
	}
	url := strings.TrimSpace(provider.Data[0].URL)
	if !isHTTPURL(url) || strings.TrimSpace(provider.Data[0].B64JSON) != "" {
		return nil, nil, upstreamError("image provider result must contain exactly one HTTP(S) URL")
	}
	var usage *dto.Usage
	if provider.Usage != nil {
		usage = &dto.Usage{}
		if n := provider.Usage.InputImages; n != nil {
			value, err := n.Int64()
			if err != nil || value < 0 || value > service.MaxImageInputs {
				return nil, nil, upstreamError("invalid image usage")
			}
			count := int(value)
			usage.InputImages = &count
		}
		if n := provider.Usage.OutputTokens; n != nil {
			value, err := n.Int64()
			if err != nil || value < 0 || value > math.MaxInt32 {
				return nil, nil, upstreamError("invalid image usage")
			}
			usage.OutputTokens, usage.CompletionTokens, usage.TotalTokens = int(value), int(value), int(value)
		}
	}
	return []string{url}, usage, nil
}
