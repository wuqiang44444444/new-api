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
func videoConfiguration(configuration *pluginruntime.SeedanceChannelConfiguration) (*pluginruntime.SeedanceVideoConfiguration, error) {
	if configuration == nil {
		return nil, minimaxplugin.ErrUnavailable
	}
	for i := range configuration.Videos {
		if configuration.Videos[i].Protocol == protocolName() {
			return &configuration.Videos[i], nil
		}
	}
	return nil, minimaxplugin.ErrUnavailable
}

// declaredMetadata resolves the per-provider-model metadata declared by the
// pinned artifact. Listed models always carry declared metadata (compile
// validation enforces this), so absence is a host contract violation.
func declaredMetadata(configuration *pluginruntime.SeedanceChannelConfiguration, providerModel string) (*pluginruntime.SeedanceVideoModelMetadata, error) {
	video, err := videoConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	if metadata, ok := video.ModelMetadata[providerModel]; ok {
		return &metadata, nil
	}
	return nil, fmt.Errorf("provider model %q has no declared metadata in the active minimax-link version", providerModel)
}

// declaredModelList reports whether the provider model is declared by the
// pinned artifact version.
func declaredModelList(configuration *pluginruntime.SeedanceChannelConfiguration) ([]string, error) {
	video, err := videoConfiguration(configuration)
	if err != nil {
		return nil, err
	}
	return video.Models, nil
}

// declaredProviderModelAuthorized reports whether the provider model belongs
// to the pinned declaration. This is validation, not a second declaration
// source: the pinned artifact version is the only model registry.
func declaredProviderModelAuthorized(configuration *pluginruntime.SeedanceChannelConfiguration, providerModel string) bool {
	video, err := videoConfiguration(configuration)
	if err != nil {
		return false
	}
	for _, model := range video.Models {
		if model == providerModel {
			return true
		}
	}
	return false
}

// metadataResolutionEnums and metadataRatioEnums project the declared open
// sets for probe and preservation checks.
func metadataResolutionEnums(metadata *pluginruntime.SeedanceVideoModelMetadata) []string {
	return metadata.Resolutions
}

func metadataRatioEnums(metadata *pluginruntime.SeedanceVideoModelMetadata) []string {
	return metadata.Ratios
}
