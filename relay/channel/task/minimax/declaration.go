package minimax

import (
	"fmt"

	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/minimaxplugin"
	"github.com/gin-gonic/gin"
)

// pinnedConfiguration returns the declaration paired with this request's
// pinned code. Missing declarations fail closed; there is no Go-side default
// table to fall back on.
func pinnedConfiguration(requestContext *gin.Context) (*pluginruntime.SeedanceChannelConfiguration, error) {
	plugin := PinnedMinimaxExtension(requestContext)
	if plugin == nil {
		return nil, minimaxplugin.ErrUnavailable
	}
	value, exists := requestContext.Get(minimaxConfigurationContextKey)
	entry, ok := value.(*minimaxplugin.CompiledVersion)
	if !exists || !ok || entry == nil || entry.Plugin != plugin || entry.Info.Configuration == nil {
		return nil, minimaxplugin.ErrUnavailable
	}
	return entry.Info.Configuration, nil
}

// videoConfiguration returns the declaration entry of this package's protocol.
func videoConfiguration(configuration *pluginruntime.SeedanceChannelConfiguration, protocol string) (*pluginruntime.SeedanceVideoConfiguration, error) {
	if configuration == nil {
		return nil, minimaxplugin.ErrUnavailable
	}
	for i := range configuration.Videos {
		if configuration.Videos[i].Protocol == protocol {
			return &configuration.Videos[i], nil
		}
	}
	return nil, minimaxplugin.ErrUnavailable
}

// declaredMetadata resolves the model's capabilities from the pinned artifact.
func declaredMetadata(configuration *pluginruntime.SeedanceChannelConfiguration, protocol, providerModel string) (*pluginruntime.SeedanceVideoModelMetadata, error) {
	video, err := videoConfiguration(configuration, protocol)
	if err != nil {
		return nil, err
	}
	if metadata, ok := video.MetadataForModel(providerModel); ok {
		return &metadata, nil
	}
	return nil, fmt.Errorf("provider model %q has no declared metadata in the active minimax-link version", providerModel)
}

// metadataResolutionEnums and metadataRatioEnums project the declared open
// sets for probe and preservation checks.
func metadataResolutionEnums(metadata *pluginruntime.SeedanceVideoModelMetadata) []string {
	return metadata.Resolutions
}

func metadataRatioEnums(metadata *pluginruntime.SeedanceVideoModelMetadata) []string {
	return metadata.Ratios
}
