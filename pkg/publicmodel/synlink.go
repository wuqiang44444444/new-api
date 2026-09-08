package publicmodel

import (
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

func synlinkVideoAPI(customerModel, providerModel string) (*dto.PublicVideoAPI, bool) {
	if !slices.Contains(dto.SynlinkVideoModels(), providerModel) {
		return nil, false
	}
	// No unverified Provider-specific duration or media-count limits are
	// advertised. Runtime still applies the common request safety bounds.
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		{Name: "content", Type: "array", Required: true, MinItems: intPointer(1)},
		{Name: "duration", Type: "integer", Minimum: intPointer(1), DefaultValue: 5},
		stringEnumParameterWithDefault("resolution", false, []string{"480p", "720p", "1080p", "4k", "4K"}, "720p"),
		stringEnumParameter("ratio", false, modelArkRatios),
		{Name: "generate_audio", Type: "boolean", DefaultValue: false},
		{Name: "watermark", Type: "boolean"},
		{Name: "return_last_frame", Type: "boolean"},
	}
	return &dto.PublicVideoAPI{
		Protocol: "modelark_v3", DocumentationPath: "/docs/api-reference/videos/modelark",
		Operations: []dto.PublicAPIOperation{
			{Operation: "create_video", Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "list_videos", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks", Supported: true},
			{Operation: "get_video", Method: http.MethodGet, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: true},
			{Operation: "delete_video", Method: http.MethodDelete, Path: "/api/v3/contents/generations/tasks/{task_id}", Supported: false},
			{Operation: "get_video_content", Method: http.MethodGet, Path: "/v1/videos/{task_id}/content", Supported: true},
		},
		Creation: dto.PublicVideoCreation{
			Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", ContentType: "application/json",
			RequiredFields: []string{"model", "content"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters,
			ContentTypes: videoContentTypes(videoSpec{allowVideos: true, allowAudios: true}),
		},
	}, true
}
