package publicmodel

import (
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

type videoSpec struct {
	minDuration, maxDuration int
	intelligentDuration      bool
	durationRequired         bool
	resolutions              []string
	resolutionRequired       bool
	freeResolution           bool
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

func modelArkVideoAPI(customerModel string, protocol dto.VideoUpstreamProtocol, spec videoSpec, allowServiceTier bool) (*dto.PublicVideoAPI, bool) {
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
	} else if spec.freeResolution {
		// The Moxing documentation enumerates no resolution values (720p is
		// only a recommendation), so the catalog publishes a free string with
		// the northbound default and the provider judges unlisted values.
		parameters = append(parameters, dto.PublicAPIParameter{Name: "resolution", Type: "string", DefaultValue: "720p"})
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
	if protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		parameters = append(parameters,
			dto.PublicAPIParameter{Name: "return_last_frame", Type: "boolean"},
			integerRangeParameter("priority", false, 0, 9),
		)
	}

	return &dto.PublicVideoAPI{
		Protocol:          "modelark_v3",
		DocumentationPath: "/docs/api-reference/videos/modelark",
		Operations: []dto.PublicAPIOperation{
			{Operation: "create_video", Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "list_videos", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "get_video", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: true},
			{Operation: "delete_video", Method: http.MethodDelete, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: protocol != dto.VideoUpstreamProtocolFunCloudModelArkV3},
			{Operation: "get_video_content", Method: http.MethodGet, Path: "/v1/videos/{task_id}/content", Supported: true},
		},
		Creation: dto.PublicVideoCreation{
			Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", ContentType: "application/json",
			RequiredFields: []string{"model", "content"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters, ContentTypes: videoContentTypes(spec),
		},
	}, true
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
