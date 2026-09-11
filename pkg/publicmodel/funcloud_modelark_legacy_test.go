package publicmodel

import "github.com/QuantumNous/new-api/relaykit/dto"

func funCloudModelArkVideoSpec(model string) (videoSpec, bool) {
	spec, ok := dto.FunCloudModelArkSpec(model)
	if !ok {
		return videoSpec{}, false
	}
	return videoSpec{
		minDuration: spec.MinDuration, maxDuration: spec.MaxDuration, intelligentDuration: spec.IntelligentDuration,
		resolutions: spec.Resolutions, ratios: modelArkRatios,
		maxImages: spec.MaxImages, maxVideos: spec.MaxVideos, maxAudios: spec.MaxAudios,
		allowVideos: true, allowAudios: true, allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
		outputFormats: []string{"mp4", "mov"},
	}, true
}
