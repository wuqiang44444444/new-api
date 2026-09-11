package publicmodel

import (
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// VideoAPIFromPlugin projects only host-defined northbound fields. Provider
// names, configuration and model IDs cannot escape through this projection.
func VideoAPIFromPlugin(customerModel string, protocol dto.VideoUpstreamProtocol, metadata jsplugin.SeedanceVideoModelMetadata, allowServiceTier bool) (*dto.PublicVideoAPI, bool) {
	spec := videoSpec{
		minDuration:         metadata.MinDuration,
		maxDuration:         metadata.MaxDuration,
		intelligentDuration: metadata.IntelligentDuration,
		durationRequired:    metadata.DurationRequired,
		resolutions:         metadata.Resolutions,
		resolutionRequired:  metadata.ResolutionRequired,
		freeResolution:      metadata.FreeResolution,
		ratios:              metadata.Ratios,
		ratioRequired:       metadata.RatioRequired,
		maxImages:           metadata.MaxImages,
		maxVideos:           metadata.MaxVideos,
		minImages:           metadata.MinImages,
		maxAudios:           metadata.MaxAudios,
		allowVideos:         metadata.AllowVideos,
		allowAudios:         metadata.AllowAudios,
		allowGenerateAudio:  metadata.AllowGenerateAudio,
		allowWatermark:      metadata.AllowWatermark,
		allowSeed:           metadata.AllowSeed,
		allowCameraFixed:    metadata.AllowCameraFixed,
		outputFormats:       metadata.OutputFormats,
		fullModelArk:        metadata.FullModelArk,
	}
	api, ok := modelArkVideoAPI(customerModel, "", spec, allowServiceTier)
	if !ok {
		return nil, false
	}
	for i := range api.Creation.Parameters {
		parameter := &api.Creation.Parameters[i]
		if parameter.Name == "duration" && metadata.OmitDurationMaximum {
			parameter.Maximum = nil
		}
		if parameter.Name == "generate_audio" && metadata.PublishGenerateAudioDefault {
			parameter.DefaultValue = metadata.DefaultGenerateAudio
		}
	}
	if metadata.AllowReturnLastFrame {
		api.Creation.Parameters = append(api.Creation.Parameters, dto.PublicAPIParameter{Name: "return_last_frame", Type: "boolean"})
	}
	if metadata.AllowPriority {
		api.Creation.Parameters = append(api.Creation.Parameters, integerRangeParameter("priority", false, 0, 9))
	}
	if metadata.DeleteVideo != nil {
		for i := range api.Operations {
			if api.Operations[i].Operation == "delete_video" {
				api.Operations[i].Supported = *metadata.DeleteVideo
			}
		}
	}
	return api, true
}
