package model

import (
	"fmt"

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

	providerModels, err := resolveSeedanceChannelProviderModels(
		channel,
		settings.VideoUpstreamProtocol,
		funCloudSeedanceProviderModels,
	)
	if err != nil {
		return err
	}

	switch settings.AssetUpstreamProtocol {
	case dto.AssetUpstreamProtocolNone:
		return nil
	case dto.AssetUpstreamProtocolFunCloudMaterial:
		for customerModel, providerModel := range providerModels {
			if providerModel == "seedance-2-5" {
				return fmt.Errorf("customer model %q resolves to FunCloud Seedance 2.5; FunCloud Seedance 2.5 does not support the FunCloud material protocol", customerModel)
			}
		}
		return nil
	default:
		return fmt.Errorf("FunCloud Seedance channels require funcloud_material or none")
	}
}
