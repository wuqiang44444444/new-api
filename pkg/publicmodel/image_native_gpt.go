package publicmodel

import "github.com/QuantumNous/new-api/relaykit/dto"

// Exact upstream profiles are used only for documentation projection, never routing.
func nativeGPTImageGenerationParameters(model string) []dto.PublicAPIParameter {
	flexibleSize, extendedQuality := false, false
	switch model {
	case "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08",
		"gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08":
		flexibleSize, extendedQuality = true, true
	case "gpt-image-2", "gpt-image-2-2026-04-21":
		flexibleSize = true
	case "gpt-image-1", "gpt-image-1-mini", "gpt-image-1.5", "chatgpt-image-latest":
	default:
		return nil
	}
	parameters := nativeGPTImageParameters(flexibleSize)
	if extendedQuality {
		for index := range parameters {
			if parameters[index].Name == "quality" {
				parameters[index].Enum = []string{"low", "medium", "high", "xhigh", "max", "auto"}
			}
		}
	}
	return parameters
}

func nativeImageEditAPI(creation dto.PublicImageCreation, providerModel string) *dto.PublicImageCreation {
	edit := creation
	edit.Path = "/v1/images/edits"
	edit.Parameters = append([]dto.PublicAPIParameter(nil), creation.Parameters...)
	if nativeGPTImageGenerationParameters(providerModel) != nil {
		edit.ContentType = "application/json"
		edit.RequiredFields = []string{"model", "prompt", "images"}
		edit.Parameters = append(edit.Parameters,
			dto.PublicAPIParameter{Name: "images", Type: "array", ItemType: "object", Required: true, MinItems: intPointer(1), MaxItems: intPointer(16), RequiredOneOf: []string{"image_url", "file_id"}},
			dto.PublicAPIParameter{Name: "images[].image_url", Type: "string"},
			dto.PublicAPIParameter{Name: "images[].file_id", Type: "string"},
			dto.PublicAPIParameter{Name: "mask", Type: "object", RequiredOneOf: []string{"image_url", "file_id"}},
			dto.PublicAPIParameter{Name: "mask.image_url", Type: "string"},
			dto.PublicAPIParameter{Name: "mask.file_id", Type: "string"},
			stringEnumParameter("input_fidelity", false, []string{"high", "low"}),
		)
		return &edit
	}
	if providerModel == "dall-e-2" || providerModel == "dall-e" {
		edit.ContentType = "multipart/form-data"
		edit.RequiredFields = []string{"model", "prompt", "image"}
		edit.Parameters = append(edit.Parameters, dto.PublicAPIParameter{Name: "image", Type: "file", Required: true}, dto.PublicAPIParameter{Name: "mask", Type: "file"})
		return &edit
	}
	// An unknown compatible model's name is not evidence of edit support.
	return nil
}
