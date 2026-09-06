package publicmodel

import (
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GeminiImageAPI 发布 gemini_image 族（Gemini 24 / Vertex 41 的
// generateContent 图片模型）的客户合同（G1 §2.1）。识别依据是管理员映射
// 后落在 imagine 登记表内的 Provider 模型，不是客户模型名。
func GeminiImageAPI(customerModel string) *dto.PublicModelAPI {
	minImages, maxImages := 1, 14
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		stringLengthParameter("prompt", true, 1, 20000),
		fixedParameterWithDefault("n", "integer", false, 1, 1),
		{Name: "size", Type: "string", DefaultValue: "auto"},
		stringEnumParameterWithDefault("response_format", false, []string{"b64_json", "url"}, "b64_json"),
		{Name: "stream", Type: "boolean", FixedValue: false, DefaultValue: false},
		userParameter(),
	}

	editParameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		stringLengthParameter("prompt", true, 1, 20000),
		fixedParameterWithDefault("n", "integer", false, 1, 1),
		{Name: "image", Type: "string"},
		{Name: "images", Type: "array", ItemType: "string", MinItems: &minImages, MaxItems: &maxImages},
		{Name: "size", Type: "string", Required: false, DefaultValue: "auto"},
		stringEnumParameterWithDefault("response_format", false, []string{"b64_json", "url"}, "b64_json"),
		{Name: "stream", Type: "boolean", FixedValue: false, DefaultValue: false},
		userParameter(),
	}

	return &dto.PublicModelAPI{Image: &dto.PublicImageAPI{
		DocumentationPath: "/docs/api-reference/images/generations",
		Operations: []dto.PublicAPIOperation{
			{Operation: "create_image", Method: http.MethodPost, Path: "/v1/images/generations", Supported: true},
			{Operation: "edit_image", Method: http.MethodPost, Path: "/v1/images/edits", Supported: true},
		},
		Creation: dto.PublicImageCreation{
			Method: http.MethodPost, Path: "/v1/images/generations", ContentType: "application/json",
			RequiredFields: []string{"model", "prompt"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters,
		},
		Edit: &dto.PublicImageCreation{
			Method: http.MethodPost, Path: "/v1/images/edits", ContentType: "application/json",
			RequiredFields: []string{"model", "prompt"}, RequiredOneOf: []string{"image", "images"}, Model: customerModel,
			AdditionalProperties: false, Parameters: editParameters,
		},
	}}
}
