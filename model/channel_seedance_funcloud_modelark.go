package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

func validateFunCloudModelArkChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	allowed := make(map[string]struct{})
	for _, name := range kitdto.FunCloudModelArkModels() {
		allowed[name] = struct{}{}
	}
	if _, err := resolveSeedanceChannelProviderModels(channel, settings.VideoUpstreamProtocol, allowed); err != nil {
		return err
	}
	switch settings.AssetUpstreamProtocol {
	case dto.AssetUpstreamProtocolNone, dto.AssetUpstreamProtocolFunCloudMaterial, dto.AssetUpstreamProtocolFunCloudHosted:
		return nil
	default:
		return fmt.Errorf("FunCloud ModelArk V3 channels require funcloud_material, funcloud_material_hosted, or none")
	}
}
