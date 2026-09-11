package seedance

import (
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func publishedVideoFixture(name string, protocol dto.VideoUpstreamProtocol, model string, tier bool) (*dto.PublicVideoAPI, bool) {
	_, info, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	if err != nil {
		panic(err)
	}
	for _, video := range info.Configuration.Videos {
		if video.Protocol == string(protocol) {
			spec, ok := video.ModelMetadata[model]
			if !ok {
				if video.DefaultModelMetadata == nil {
					return nil, false
				}
				spec = *video.DefaultModelMetadata
			}
			return publicmodel.VideoAPIFromPlugin(name, protocol, spec, tier)
		}
	}
	return nil, false
}
