package seedance

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Provider defaults used by the billing probe are read from the same artifact
// pinned for conversion. Historical Go definitions are only used by protocols
// that have not moved to the plugin.
func (a *TaskAdaptor) pinnedProviderSpec(c *gin.Context, model string) (providerModelSpec, bool, error) {
	if !SeedanceExtensionProtocolMigrated(a.protocol) {
		return providerModelSpec{}, false, nil
	}
	configuration, err := PinnedSeedanceConfiguration(c)
	if err != nil {
		return providerModelSpec{}, false, err
	}
	if configuration == nil {
		return providerModelSpec{}, false, nil
	}
	for _, video := range configuration.Videos {
		if video.Protocol != string(a.protocol) {
			continue
		}
		metadata, ok := video.ModelMetadata[model]
		if !ok && video.DefaultModelMetadata != nil {
			metadata = *video.DefaultModelMetadata
			ok = true
		}
		if !ok {
			return providerModelSpec{}, false, nil
		}
		return providerModelSpec{defaultDuration: metadata.DefaultDuration, minDuration: metadata.MinDuration, maxDuration: metadata.MaxDuration, intelligentDuration: metadata.IntelligentDurationSeconds, allowIntelligentDuration: metadata.IntelligentDuration, resolutions: stringSet(metadata.Resolutions...), maxImages: metadata.MaxImages, maxVideos: metadata.MaxVideos, maxAudios: metadata.MaxAudios, maxTotalMedia: metadata.MaxTotalMedia, allowVideos: metadata.AllowVideos, allowAudios: metadata.AllowAudios, allowAudioOnly: metadata.AllowAudioOnly, defaultGenerateAudio: metadata.DefaultGenerateAudio, outputFormats: stringSet(metadata.OutputFormats...)}, true, nil
	}
	return providerModelSpec{}, false, fmt.Errorf("video protocol is absent from the pinned plugin configuration")
}
