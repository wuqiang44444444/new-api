package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func resolveSeedanceChannelProviderModels(
	channel *Channel,
	protocol dto.VideoUpstreamProtocol,
	supportedProviderModels map[string]struct{},
) (map[string]string, error) {
	if channel == nil {
		return nil, fmt.Errorf("Seedance channel is required")
	}
	models := channel.GetModels()
	if len(models) == 0 {
		return nil, fmt.Errorf("Seedance channels require at least one customer model")
	}

	mapping := make(map[string]string)
	modelMapping := strings.TrimSpace(channel.GetModelMapping())
	if modelMapping != "" && modelMapping != "{}" {
		if err := common.UnmarshalJsonStr(modelMapping, &mapping); err != nil {
			return nil, fmt.Errorf("Seedance channel model_mapping must be a JSON object")
		}
	}

	resolved := make(map[string]string, len(models))
	for _, modelName := range models {
		customerModel := strings.TrimSpace(modelName)
		if customerModel == "" {
			return nil, fmt.Errorf("Seedance channels require non-empty customer model names")
		}
		if _, duplicate := resolved[customerModel]; duplicate {
			return nil, fmt.Errorf("Seedance customer model %q is duplicated", customerModel)
		}
		providerModel, _, err := ResolveModelMapping(customerModel, mapping)
		if err != nil {
			return nil, fmt.Errorf("model_mapping for customer model %q is invalid: %w", customerModel, err)
		}
		if _, supported := supportedProviderModels[providerModel]; !supported {
			return nil, fmt.Errorf(
				"model_mapping for customer model %q resolves to Provider model %q unsupported by video protocol %s",
				customerModel,
				providerModel,
				protocol,
			)
		}
		resolved[customerModel] = providerModel
	}
	return resolved, nil
}
