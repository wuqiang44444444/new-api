package model

import (
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func seedancePublicModelAPIFromPlugin(configuration *jsplugin.SeedanceChannelConfiguration, modelName string, protocol dto.VideoUpstreamProtocol, providerModel string, allowServiceTier bool, assetProtocol dto.AssetUpstreamProtocol, ttl int64, reuseScope string) (dto.PublicModelAPI, bool) {
	if configuration != nil {
		for _, video := range configuration.Videos {
			if video.Protocol != string(protocol) {
				continue
			}
			metadata, exists := video.MetadataForModel(providerModel)
			if !exists {
				return dto.PublicModelAPI{}, false
			}
			api, ok := publicmodel.VideoAPIFromPlugin(modelName, protocol, metadata, allowServiceTier)
			if !ok {
				return dto.PublicModelAPI{}, false
			}
			hosted := publicHostedAssetAPI(modelName)
			if assetProtocol == "" || assetProtocol == dto.AssetUpstreamProtocolNone || assetProtocol == dto.AssetUpstreamProtocolFunCloudHosted {
				return dto.PublicModelAPI{Video: api, Assets: &hosted}, true
			}
			assetDeclaration := configuration.Asset(string(assetProtocol))
			if assetDeclaration == nil {
				return dto.PublicModelAPI{}, false
			}
			assets := seedancePublicAssetAPIWithDeclaration(modelName, assetProtocol, ttl, reuseScope, assetDeclaration)
			return dto.PublicModelAPI{Video: api, Assets: &assets, HostedAssets: &hosted}, true
		}
	}
	return dto.PublicModelAPI{}, false
}
