package model

import (
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func seedancePublicModelAPI(name string, video dto.VideoUpstreamProtocol, provider string, tier bool, asset dto.AssetUpstreamProtocol, ttl int64, scope string) (dto.PublicModelAPI, bool) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	if err != nil {
		panic(err)
	}
	return seedancePublicModelAPIFromPlugin(info.Configuration, name, video, provider, tier, asset, ttl, scope)
}
func seedancePublicAssetAPI(name string, protocol dto.AssetUpstreamProtocol, ttl int64, scope string) dto.PublicAssetAPI {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	if err != nil {
		panic(err)
	}
	return seedancePublicAssetAPIWithDeclaration(name, protocol, ttl, scope, info.Configuration.Asset(string(protocol)))
}
