package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

func validateSynlinkChannel(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 {
		return nil
	}
	allowed := make(map[string]struct{})
	for _, name := range kitdto.SynlinkVideoModels() {
		allowed[name] = struct{}{}
	}
	if _, err := resolveSeedanceChannelProviderModels(channel, settings.VideoUpstreamProtocol, allowed); err != nil {
		return err
	}
	if settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolNone && settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolFunCloudHosted {
		return fmt.Errorf("Synlink requires platform-hosted images or no asset protocol")
	}
	return nil
}
