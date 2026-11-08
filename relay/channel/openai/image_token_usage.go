package openai

import (
	"math"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
)

func recordImageTokenUsage(usage *dto.Usage, body []byte) {
	inputPath := "usage.prompt_tokens_details"
	if usage.InputTokensDetails != nil {
		inputPath = "usage.input_tokens_details"
	}
	outputPath := "usage.completion_tokens_details"
	if usage.OutputTokensDetails != nil {
		outputPath = "usage.output_tokens_details"
	}
	textInput, imageInput := reportedImageTokenDetails(gjson.GetBytes(body, inputPath), usage.PromptTokens)
	textOutput, imageOutput := reportedImageTokenDetails(gjson.GetBytes(body, outputPath), usage.CompletionTokens)
	if textInput != nil || imageInput != nil || textOutput != nil || imageOutput != nil {
		usage.ImageTokenUsage = &dto.ImageTokenUsage{
			TextInput: textInput, ImageInput: imageInput,
			TextOutput: textOutput, ImageOutput: imageOutput,
		}
	}
}

func reportedImageTokenDetails(details gjson.Result, total int) (text, image *int) {
	text = reportedImageTokenCount(details.Get("text_tokens"), total)
	image = reportedImageTokenCount(details.Get("image_tokens"), total)
	if text != nil && image != nil && *text > total-*image {
		return nil, nil
	}
	return text, image
}

func reportedImageTokenCount(value gjson.Result, total int) *int {
	if value.Type != gjson.Number || value.Num < 0 || value.Num > math.MaxInt32 ||
		math.Trunc(value.Num) != value.Num || value.Num > float64(total) {
		return nil
	}
	count := int(value.Num)
	return &count
}
