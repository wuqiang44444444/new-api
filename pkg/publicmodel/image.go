package publicmodel

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

var allImageAspectRatios = []string{
	"auto", "1:1", "1:4", "16:9", "1:8", "21:9", "2:3", "3:2", "3:4", "4:1", "4:3", "4:5", "5:4", "8:1", "9:16",
}

var seedreamLiteAspectRatios = []string{"1:1", "4:3", "3:4", "16:9", "9:16", "2:3", "3:2", "21:9"}
var seedreamProAspectRatios = []string{"1:1", "4:3", "3:4", "16:9", "9:16", "2:3", "3:2"}

func ImageAPI(customerModel string, protocol dto.ImageUpstreamProtocol, providerModel string) (*dto.PublicModelAPI, bool) {
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		stringLengthParameter("prompt", true, 1, 0),
		fixedParameterWithDefault("n", "integer", false, 1, 1),
		stringEnumParameterWithDefault("response_format", false, []string{"url"}, "url"),
	}

	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		switch providerModel {
		case constant.FunCloudImageProviderModelNanoBanana2Lite:
			parameters[1].MaxLength = intPointer(20000)
			parameters = append(parameters, stringEnumParameter("extra_fields.aspect_ratio", false, allImageAspectRatios))
		case constant.FunCloudImageProviderModelNanoBanana2:
			parameters[1].MaxLength = intPointer(20000)
			parameters = append(parameters,
				stringEnumParameterWithDefault("size", false, []string{"1K"}, "1K"),
				stringEnumParameter("output_format", false, []string{"jpg", "png"}),
				stringEnumParameter("extra_fields.aspect_ratio", false, allImageAspectRatios),
				stringEnumParameter("extra_fields.resolution", false, []string{"1K"}),
				stringEnumParameter("extra_fields.output_format", false, []string{"jpg", "png"}),
			)
		case constant.FunCloudImageProviderModelSeedream5Lite:
			parameters[1].MinLength = intPointer(3)
			parameters[1].MaxLength = intPointer(3000)
			parameters = append(parameters,
				stringEnumParameterWithDefault("size", false, []string{"2K"}, "2K"),
				stringEnumParameterWithDefault("quality", false, []string{"basic"}, "basic"),
				stringEnumParameter("extra_fields.aspect_ratio", false, seedreamLiteAspectRatios),
				stringEnumParameter("extra_fields.quality", false, []string{"basic"}),
			)
		case constant.FunCloudImageProviderModelSeedream5Pro:
			parameters[1].MinLength = intPointer(3)
			parameters[1].MaxLength = intPointer(3000)
			parameters = append(parameters,
				stringEnumParameterWithDefault("size", false, []string{"1K"}, "1K"),
				stringEnumParameterWithDefault("quality", false, []string{"basic"}, "basic"),
				stringEnumParameter("extra_fields.aspect_ratio", false, seedreamProAspectRatios),
				stringEnumParameter("extra_fields.quality", false, []string{"basic"}),
			)
		default:
			return nil, false
		}
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		fixedSize, ok := constant.MoxingImageFixedSize(providerModel)
		if !ok {
			return nil, false
		}
		parameters[1].MaxLength = intPointer(3000)
		parameters = append(parameters, stringEnumParameterWithDefault("size", false, []string{fixedSize}, fixedSize))
	default:
		return nil, false
	}

	return imageModelAPI(customerModel, parameters), true
}

func NativeImageAPI(customerModel string) *dto.PublicModelAPI {
	parameters := []dto.PublicAPIParameter{
		fixedParameter("model", "string", true, customerModel),
		stringLengthParameter("prompt", true, 1, nativeImagePromptMaxLength(customerModel)),
	}

	switch customerModel {
	case "dall-e-2", "dall-e":
		parameters = append(parameters,
			integerRangeParameterWithDefault("n", false, 1, 10, 1),
			stringEnumParameterWithDefault("size", false, []string{"256x256", "512x512", "1024x1024"}, "1024x1024"),
			stringEnumParameterWithDefault("response_format", false, []string{"url", "b64_json"}, "url"),
			userParameter(),
		)
	case "dall-e-3":
		parameters = append(parameters,
			fixedParameterWithDefault("n", "integer", false, 1, 1),
			stringEnumParameterWithDefault("size", false, []string{"1024x1024", "1024x1792", "1792x1024"}, "1024x1024"),
			stringEnumParameterWithDefault("quality", false, []string{"standard", "hd"}, "standard"),
			stringEnumParameterWithDefault("response_format", false, []string{"url", "b64_json"}, "url"),
			stringEnumParameterWithDefault("style", false, []string{"vivid", "natural"}, "vivid"),
			userParameter(),
		)
	case "gpt-image-2":
		parameters = append(parameters, nativeGPTImageParameters(true)...)
	case "gpt-image-1", "gpt-image-1-mini", "gpt-image-1.5", "chatgpt-image-latest":
		parameters = append(parameters, nativeGPTImageParameters(false)...)
	default:
		// Other OpenAI-compatible image models only publish the stable gateway
		// surface. Provider-specific fields stay absent rather than being guessed.
		parameters = append(parameters,
			integerRangeParameter("n", false, 1, dto.MaxImageN),
			dto.PublicAPIParameter{Name: "size", Type: "string"},
			dto.PublicAPIParameter{Name: "quality", Type: "string"},
			dto.PublicAPIParameter{Name: "response_format", Type: "string"},
			userParameter(),
		)
	}
	return imageModelAPI(customerModel, parameters)
}

func nativeImagePromptMaxLength(model string) int {
	switch model {
	case "dall-e-2", "dall-e":
		return 1000
	case "dall-e-3":
		return 4000
	case "gpt-image-2", "gpt-image-1", "gpt-image-1-mini", "gpt-image-1.5", "chatgpt-image-latest":
		return 32000
	default:
		return 0
	}
}

func nativeGPTImageParameters(flexibleSize bool) []dto.PublicAPIParameter {
	size := dto.PublicAPIParameter{Name: "size", Type: "string", DefaultValue: "auto"}
	if !flexibleSize {
		size.Enum = []string{"1024x1024", "1024x1536", "1536x1024", "auto"}
	}
	return []dto.PublicAPIParameter{
		integerRangeParameterWithDefault("n", false, 1, 10, 1),
		size,
		stringEnumParameterWithDefault("quality", false, []string{"low", "medium", "high", "auto"}, "auto"),
		stringEnumParameterWithDefault("background", false, []string{"transparent", "opaque", "auto"}, "auto"),
		stringEnumParameterWithDefault("moderation", false, []string{"low", "auto"}, "auto"),
		stringEnumParameterWithDefault("output_format", false, []string{"png", "webp", "jpeg"}, "png"),
		integerRangeParameter("output_compression", false, 0, 100),
		integerRangeParameter("partial_images", false, 0, 3),
		{Name: "stream", Type: "boolean", DefaultValue: false},
		userParameter(),
	}
}

func userParameter() dto.PublicAPIParameter {
	return dto.PublicAPIParameter{Name: "user", Type: "string"}
}

func imageModelAPI(customerModel string, parameters []dto.PublicAPIParameter) *dto.PublicModelAPI {
	return &dto.PublicModelAPI{Image: &dto.PublicImageAPI{
		DocumentationPath: "/docs/api-reference/images/generations",
		Operations: []dto.PublicAPIOperation{{
			Operation: "create_image", Method: http.MethodPost, Path: "/v1/images/generations", Supported: true,
		}},
		Creation: dto.PublicImageCreation{
			Method: http.MethodPost, Path: "/v1/images/generations", ContentType: "application/json",
			RequiredFields: []string{"model", "prompt"}, Model: customerModel,
			AdditionalProperties: false, Parameters: parameters,
		},
	}}
}

func fixedParameter(name, parameterType string, required bool, value any) dto.PublicAPIParameter {
	return dto.PublicAPIParameter{Name: name, Type: parameterType, Required: required, FixedValue: value}
}

func fixedParameterWithDefault(name, parameterType string, required bool, value, defaultValue any) dto.PublicAPIParameter {
	parameter := fixedParameter(name, parameterType, required, value)
	parameter.DefaultValue = defaultValue
	return parameter
}

func stringEnumParameter(name string, required bool, values []string) dto.PublicAPIParameter {
	return dto.PublicAPIParameter{Name: name, Type: "string", Required: required, Enum: values}
}

func stringEnumParameterWithDefault(name string, required bool, values []string, defaultValue string) dto.PublicAPIParameter {
	parameter := stringEnumParameter(name, required, values)
	parameter.DefaultValue = defaultValue
	return parameter
}

func stringLengthParameter(name string, required bool, minimum, maximum int) dto.PublicAPIParameter {
	parameter := dto.PublicAPIParameter{Name: name, Type: "string", Required: required}
	if minimum > 0 {
		parameter.MinLength = intPointer(minimum)
	}
	if maximum > 0 {
		parameter.MaxLength = intPointer(maximum)
	}
	return parameter
}

func integerRangeParameter(name string, required bool, minimum, maximum int) dto.PublicAPIParameter {
	return dto.PublicAPIParameter{Name: name, Type: "integer", Required: required, Minimum: intPointer(minimum), Maximum: intPointer(maximum)}
}

func integerRangeParameterWithDefault(name string, required bool, minimum, maximum, defaultValue int) dto.PublicAPIParameter {
	parameter := integerRangeParameter(name, required, minimum, maximum)
	parameter.DefaultValue = defaultValue
	return parameter
}

func intPointer(value int) *int { return &value }
