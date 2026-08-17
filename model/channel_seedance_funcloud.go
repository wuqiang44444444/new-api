package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

var funCloudSeedanceProviderModels = map[string]struct{}{
	"seedance-2":      {},
	"seedance-2-fast": {},
	"seedance-2-mini": {},
	"seedance-2-5":    {},
}

func validateFunCloudSeedanceChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolFunCloudSeedance {
		if settings.AssetUpstreamProtocol == dto.AssetUpstreamProtocolFunCloudMaterial {
			return fmt.Errorf("FunCloud material protocol requires the FunCloud Seedance video protocol")
		}
		return nil
	}

	models := channel.GetModels()
	if len(models) != 1 {
		return fmt.Errorf("FunCloud Seedance channels require exactly one customer model")
	}
	customerModel := strings.TrimSpace(models[0])
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping); err != nil {
		return fmt.Errorf("FunCloud Seedance channels require one exact model_mapping entry")
	}
	providerModel := strings.TrimSpace(mapping[customerModel])
	if len(mapping) != 1 || providerModel == "" {
		return fmt.Errorf("model_mapping must contain exactly one entry for customer model %q", customerModel)
	}
	if _, ok := funCloudSeedanceProviderModels[providerModel]; !ok {
		return fmt.Errorf("model_mapping target for %q is not supported by the FunCloud Seedance protocol", customerModel)
	}

	switch settings.AssetUpstreamProtocol {
	case dto.AssetUpstreamProtocolNone:
		return nil
	case dto.AssetUpstreamProtocolFunCloudMaterial:
		if providerModel == "seedance-2-5" {
			return fmt.Errorf("FunCloud Seedance 2.5 does not support the FunCloud material protocol")
		}
		return nil
	default:
		return fmt.Errorf("FunCloud Seedance channels require funcloud_material or none")
	}
}
