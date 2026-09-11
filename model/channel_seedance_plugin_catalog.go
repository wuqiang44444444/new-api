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
			metadata, exists := video.ModelMetadata[providerModel]
			if !exists {
				if video.DefaultModelMetadata == nil {
					return dto.PublicModelAPI{}, false
				}
				metadata = *video.DefaultModelMetadata
			}
			api, ok := publicmodel.VideoAPIFromPlugin(modelName, protocol, metadata, allowServiceTier)
			if !ok {
				return dto.PublicModelAPI{}, false
			}
			assetDeclaration := configuration.Asset(string(assetProtocol))
			if assetProtocol == "" {
				assetDeclaration = configuration.Asset("none")
			}
			if assetDeclaration == nil {
				return dto.PublicModelAPI{}, false
			}
			assets := seedancePublicAssetAPIWithDeclaration(modelName, assetProtocol, ttl, reuseScope, assetDeclaration)
			return dto.PublicModelAPI{Video: api, Assets: &assets}, true
		}
	}
	return dto.PublicModelAPI{}, false
}
