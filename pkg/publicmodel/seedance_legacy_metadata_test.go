package publicmodel

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"strings"
)

var modelArkRatios = []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}
var fixedVideoRatios = []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}

func VideoAPI(customerModel string, protocol dto.VideoUpstreamProtocol, providerModel string, allowServiceTier bool) (*dto.PublicVideoAPI, bool) {
	if protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
		return synlinkVideoAPI(customerModel, strings.TrimSpace(providerModel))
	}
	spec, ok := publicVideoSpec(protocol, strings.TrimSpace(providerModel))
	if !ok {
		return nil, false
	}
	return modelArkVideoAPI(customerModel, protocol, spec, allowServiceTier)
}

func publicVideoSpec(protocol dto.VideoUpstreamProtocol, model string) (videoSpec, bool) {
	switch protocol {
	case dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus:
		if spec, ok := officialVideoSpec(model); ok {
			return spec, true
		}
		return videoSpec{
			minDuration: 1, maxDuration: 60, intelligentDuration: true,
			resolutions: []string{"480p", "720p", "1080p", "4K"}, ratios: modelArkRatios,
			allowVideos: true, allowAudios: true, allowGenerateAudio: true, allowWatermark: true,
			allowSeed: true, allowCameraFixed: true, fullModelArk: true,
		}, true
	case dto.VideoUpstreamProtocolModelArkV3CMCC:
		if model != "doubao-seedance-2.0" {
			return videoSpec{}, false
		}
		return videoSpec{
			minDuration: 4, maxDuration: 15, resolutions: []string{"480p", "720p", "1080p"},
			ratios: []string{"16:9", "9:16", "1:1"}, allowVideos: true, allowAudios: true, allowGenerateAudio: true, allowWatermark: true,
		}, true
	case dto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
		if model != "doubao-seedance-2-0-260128" {
			return videoSpec{}, false
		}
		return videoSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: true,
			resolutions: []string{"480p", "720p", "1080p"}, ratios: modelArkRatios,
			allowVideos: true, allowAudios: true,
			allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
		}, true
	case dto.VideoUpstreamProtocolMoxingModelArkV1:
		if contract, ok := dto.MoxingVideoModelContractFor(model); ok {
			return moxingPublicVideoSpec(contract), true
		}
		return videoSpec{}, false
	case dto.VideoUpstreamProtocolFunCloudModelArkV3:
		return funCloudModelArkVideoSpec(model)
	case dto.VideoUpstreamProtocolFeicaiVideosV1:
		if spec, ok := feicaiVideoSpec(model); ok {
			return spec, true
		}
		return videoSpec{}, false
	case dto.VideoUpstreamProtocolArkMediaV1:
		return videoSpec{
			minDuration: 1, maxDuration: 60, intelligentDuration: true,
			resolutions: []string{"480p", "720p", "1080p"}, ratios: modelArkRatios,
			allowVideos: true, allowAudios: true, allowGenerateAudio: true, allowWatermark: true,
			allowSeed: true, allowCameraFixed: true, fullModelArk: true,
		}, true
	}
	return videoSpec{}, false
}

func officialVideoSpec(model string) (videoSpec, bool) {
	base := videoSpec{
		minDuration: 4, maxDuration: 15, resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
		maxImages: 9, maxVideos: 3, maxAudios: 3, allowVideos: true, allowAudios: true,
		allowGenerateAudio: true, allowWatermark: true,
	}
	switch model {
	case "doubao-seedance-2-0-260128", "dreamina-seedance-2-0-260128":
		base.resolutions = []string{"480p", "720p", "1080p", "4K"}
		return base, true
	case "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615",
		"dreamina-seedance-2-0-fast-260128", "dreamina-seedance-2-0-mini-260615":
		return base, true
	case "doubao-seedance-2-5-260628", "dreamina-seedance-2-5-260628":
		base.maxDuration = 30
		base.intelligentDuration = true
		base.maxImages, base.maxVideos, base.maxAudios = 30, 10, 10
		base.allowSeed = true
		base.outputFormats = []string{"mp4", "mov"}
		return base, true
	default:
		return videoSpec{}, false
	}
}

// moxingPublicVideoSpec supplies the Go reference for plugin migration tests.
func moxingPublicVideoSpec(contract dto.MoxingVideoModelContract) videoSpec {
	return videoSpec{
		minDuration:         contract.MinDurationSeconds,
		maxDuration:         contract.MaxDurationSeconds,
		intelligentDuration: contract.IntelligentDurationSeconds > 0,
		freeResolution:      len(contract.Resolutions) == 0,
		resolutions:         contract.Resolutions,
		ratios:              modelArkRatios,
		maxImages:           contract.MaxImages,
		maxVideos:           contract.MaxVideos,
		maxAudios:           contract.MaxAudios,
		allowVideos:         contract.AllowReferenceVideos,
		allowAudios:         contract.AllowReferenceAudios,
		allowGenerateAudio:  true,
		allowWatermark:      true,
		allowSeed:           true,
		allowCameraFixed:    true,
		fullModelArk:        true,

		suggestedResolutions: contract.SuggestedResolutions,
		omitOutputFormat:     contract.OmitOutputFormat,
	}
}

func feicaiVideoSpec(model string) (videoSpec, bool) {
	spec := videoSpec{
		minDuration: 4, maxDuration: 15, durationRequired: true,
		resolutionRequired: true, ratioRequired: true, ratios: fixedVideoRatios,
		maxImages: 9, maxAudios: 3,
	}
	switch model {
	case "seedance-2.0-vip-720p-mini-azhw", "seedance-2.0-vip-720p-fast-azhw", "seedance-2.0-933-720p-azhw", "seedance-2.0-vip-720p-azhw":
		spec.resolutions = []string{"720p"}
	case "seedance2.0-sd2":
		spec.minDuration = 11
		spec.minImages = 1
		spec.maxAudios = 0
		spec.resolutions = []string{"720p"}
		spec.ratios = []string{"16:9", "9:16"}
	case "seedance-2.0-933-1080p-azhw", "seedance-2.0-vip-1080p-azhw":
		spec.resolutions = []string{"1080p"}
	case "seedance-2.0-933-4k-azhw", "seedance-2.0-vip-4k-azhw":
		spec.resolutions = []string{"4k"}
	case "seedance-933-pro-pi":
		spec.minDuration = 15
		spec.maxDuration = 15
		spec.maxVideos = 3
		spec.allowVideos = true
		spec.resolutions = []string{"720p"}
	default:
		return videoSpec{}, false
	}
	if spec.maxAudios > 0 {
		spec.allowAudios = true
	}
	return spec, true
}
