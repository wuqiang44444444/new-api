package publicmodel

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

var modelArkRatios = []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"}
var fixedVideoRatios = []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}

type videoSpec struct {
	minDuration, maxDuration int
	intelligentDuration      bool
	durationRequired         bool
	resolutions              []string
	resolutionRequired       bool
	ratios                   []string
	ratioRequired            bool
	maxImages, maxVideos     int
	minImages, maxAudios     int
	allowVideos, allowAudios bool
	allowGenerateAudio       bool
	allowWatermark           bool
	allowSeed                bool
	allowCameraFixed         bool
	outputFormats            []string
	fullModelArk             bool
}

func NativeVideoAPI(customerModel string) *dto.PublicModelAPI {
	sizes := []string{"720x1280", "1280x720"}
	if customerModel == "sora-2-pro" {
		sizes = append(sizes, "1792x1024", "1024x1792")
	}
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		stringLengthParameter("prompt", true, 1, 32000),
		stringEnumParameterWithDefault("seconds", false, []string{"4", "8", "12"}, "4"),
		stringEnumParameterWithDefault("size", false, sizes, "720x1280"),
		{Name: "input_reference", Type: "object"},
	}
	return &dto.PublicModelAPI{Video: &dto.PublicVideoAPI{
		Protocol:          "openai_videos",
		DocumentationPath: "/docs/api-reference/videos/openai",
		Operations: []dto.PublicAPIOperation{
			{Operation: "create_video", Method: http.MethodPost, Path: "/v1/videos", Supported: true},
			{Operation: "get_video", Method: http.MethodGet, Path: "/v1/videos/{task_id}", Supported: true},
			{Operation: "remix_video", Method: http.MethodPost, Path: "/v1/videos/{task_id}/remix", Supported: true},
			{Operation: "get_video_content", Method: http.MethodGet, Path: "/v1/videos/{task_id}/content", Supported: true},
		},
		Creation: dto.PublicVideoCreation{
			Method: http.MethodPost, Path: "/v1/videos", ContentType: "application/json",
			RequiredFields: []string{"model", "prompt"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters, ContentTypes: []dto.PublicVideoContentType{},
		},
	}}
}

func VideoAPI(customerModel string, protocol dto.VideoUpstreamProtocol, providerModel string, allowServiceTier bool) (*dto.PublicVideoAPI, bool) {
	spec, ok := publicVideoSpec(protocol, strings.TrimSpace(providerModel))
	if !ok {
		return nil, false
	}
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		{Name: "content", Type: "array", Required: true, MinItems: intPointer(1)},
	}
	if spec.minDuration > 0 {
		duration := integerRangeParameter("duration", spec.durationRequired, spec.minDuration, spec.maxDuration)
		if !spec.durationRequired {
			duration.DefaultValue = 5
		}
		if spec.intelligentDuration {
			duration.SpecialValues = []int{-1}
		}
		parameters = append(parameters, duration)
	}
	if len(spec.resolutions) > 0 {
		resolution := stringEnumParameter("resolution", spec.resolutionRequired, spec.resolutions)
		if spec.resolutionRequired && len(spec.resolutions) == 1 {
			resolution.FixedValue = spec.resolutions[0]
		} else if !spec.resolutionRequired {
			resolution.DefaultValue = "720p"
		}
		parameters = append(parameters, resolution)
	}
	if len(spec.ratios) > 0 {
		parameters = append(parameters, stringEnumParameter("ratio", spec.ratioRequired, spec.ratios))
	}
	if spec.allowGenerateAudio {
		parameters = append(parameters, dto.PublicAPIParameter{Name: "generate_audio", Type: "boolean"})
	}
	if spec.allowWatermark {
		parameters = append(parameters, dto.PublicAPIParameter{Name: "watermark", Type: "boolean"})
	}
	if spec.allowSeed {
		seed := integerRangeParameter("seed", false, -1, 1<<31-1)
		parameters = append(parameters, seed)
	}
	if spec.allowCameraFixed {
		parameters = append(parameters, dto.PublicAPIParameter{Name: "camera_fixed", Type: "boolean"})
	}
	if len(spec.outputFormats) > 0 {
		parameters = append(parameters, stringEnumParameter("output_format", false, spec.outputFormats))
	}
	if spec.fullModelArk {
		parameters = append(parameters, fullModelArkParameters(allowServiceTier)...)
	}

	return &dto.PublicVideoAPI{
		Protocol:          "modelark_v3",
		DocumentationPath: "/docs/api-reference/videos/modelark",
		Operations: []dto.PublicAPIOperation{
			{Operation: "create_video", Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "list_videos", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "get_video", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: true},
			{Operation: "delete_video", Method: http.MethodDelete, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: true},
			{Operation: "get_video_content", Method: http.MethodGet, Path: "/v1/videos/{task_id}/content", Supported: true},
		},
		Creation: dto.PublicVideoCreation{
			Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", ContentType: "application/json",
			RequiredFields: []string{"model", "content"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters, ContentTypes: videoContentTypes(spec),
		},
	}, true
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
			ratios: []string{"16:9", "9:16", "1:1"}, allowGenerateAudio: true, allowWatermark: true,
		}, true
	case dto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
		if model != "doubao-seedance-2-0-260128" {
			return videoSpec{}, false
		}
		return videoSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: true,
			resolutions: []string{"480p", "720p", "1080p"}, ratios: modelArkRatios,
			allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
		}, true
	case dto.VideoUpstreamProtocolMoxingMediaTaskV1:
		if model != "doubao-seedance-2-0-260128" {
			return videoSpec{}, false
		}
		return videoSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: true,
			resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
			allowVideos: true, allowAudios: true, allowGenerateAudio: true, allowWatermark: true,
		}, true
	case dto.VideoUpstreamProtocolMoxingModelArkV1:
		switch model {
		case "doubao-seedance-2-0-fast-260128", "doubao-seedance-2-0-mini-260615":
			return videoSpec{
				minDuration: 4, maxDuration: 15, intelligentDuration: true,
				resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
				maxImages: 9, maxVideos: 3, maxAudios: 3, allowVideos: true, allowAudios: true,
				allowGenerateAudio: true, allowWatermark: true,
			}, true
		case "doubao-seedance-2-5-260628":
			return videoSpec{
				minDuration: 4, maxDuration: 30, intelligentDuration: true,
				resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
				maxImages: 30, maxVideos: 10, maxAudios: 10, allowVideos: true, allowAudios: true,
				allowGenerateAudio: true, allowWatermark: true, allowSeed: true, outputFormats: []string{"mp4", "mov"},
			}, true
		}
	case dto.VideoUpstreamProtocolFunCloudSeedance:
		switch model {
		case "seedance-2":
			return videoSpec{
				minDuration: 4, maxDuration: 15, resolutions: []string{"480p", "720p", "1080p"}, ratios: modelArkRatios,
				maxImages: 3, maxVideos: 1, maxAudios: 1, allowVideos: true, allowAudios: true,
				allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
			}, true
		case "seedance-2-fast", "seedance-2-mini":
			return videoSpec{
				minDuration: 4, maxDuration: 15, resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
				maxImages: 3, maxVideos: 1, maxAudios: 1, allowVideos: true, allowAudios: true,
				allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
			}, true
		case "seedance-2-5":
			return videoSpec{
				minDuration: 4, maxDuration: 30, intelligentDuration: true,
				resolutions: []string{"480p", "720p"}, ratios: modelArkRatios,
				maxImages: 9, maxVideos: 3, maxAudios: 3, allowVideos: true, allowAudios: true,
				allowGenerateAudio: true, allowWatermark: true, allowSeed: true, allowCameraFixed: true,
			}, true
		}
		return videoSpec{}, false
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

func fullModelArkParameters(allowServiceTier bool) []dto.PublicAPIParameter {
	parameters := []dto.PublicAPIParameter{
		{Name: "callback_url", Type: "string"},
		stringEnumParameter("output_format", false, []string{"mp4", "mov"}),
		{Name: "return_last_frame", Type: "boolean"},
		integerRangeParameter("execution_expires_after", false, 3600, 259200),
		{Name: "draft", Type: "boolean"},
		{Name: "tools", Type: "array"},
		{Name: "safety_identifier", Type: "string"},
		integerRangeParameter("priority", false, 0, 9),
	}
	frames := integerRangeParameter("frames", false, 29, 289)
	frames.SpecialValues = []int{29, 33, 37, 41, 45, 49, 53, 57, 61, 65, 69, 73, 77, 81, 85, 89, 93, 97, 101, 105, 109, 113, 117, 121, 125, 129, 133, 137, 141, 145, 149, 153, 157, 161, 165, 169, 173, 177, 181, 185, 189, 193, 197, 201, 205, 209, 213, 217, 221, 225, 229, 233, 237, 241, 245, 249, 253, 257, 261, 265, 269, 273, 277, 281, 285, 289}
	parameters = append(parameters, frames)
	if allowServiceTier {
		parameters = append(parameters, stringEnumParameter("service_tier", false, []string{"default", "flex"}))
	}
	return parameters
}

func videoContentTypes(spec videoSpec) []dto.PublicVideoContentType {
	textMinimum := 0
	imageRoles := []string{"first_frame", "last_frame", "reference_image"}
	if spec.durationRequired && spec.resolutionRequired && spec.ratioRequired {
		textMinimum = 1
		imageRoles = []string{"reference_image"}
	}
	content := []dto.PublicVideoContentType{{Type: "text", RequiredFields: []string{"type", "text"}, MinItems: textMinimum}}
	content = append(content, dto.PublicVideoContentType{
		Type: "image_url", Roles: imageRoles,
		RequiredFields: []string{"type", "role", "image_url.url"}, MinItems: spec.minImages, MaxItems: spec.maxImages,
	})
	if spec.allowVideos || spec.fullModelArk {
		content = append(content, dto.PublicVideoContentType{
			Type: "video_url", Roles: []string{"reference_video"}, RequiredFields: []string{"type", "role", "video_url.url"}, MaxItems: spec.maxVideos,
		})
	}
	if spec.allowAudios || spec.fullModelArk {
		content = append(content, dto.PublicVideoContentType{
			Type: "audio_url", Roles: []string{"reference_audio"}, RequiredFields: []string{"type", "role", "audio_url.url"}, MaxItems: spec.maxAudios,
		})
	}
	return content
}
