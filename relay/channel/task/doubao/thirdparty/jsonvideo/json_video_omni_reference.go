package jsonvideo

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

type omniReferenceRequest struct {
	Model         string   `json:"model"`
	Prompt        string   `json:"prompt"`
	Duration      int      `json:"duration"`
	Ratio         string   `json:"ratio"`
	ReferenceMode string   `json:"reference_mode"`
	InputImages   []string `json:"input_images,omitempty"`
	AudioURLList  []string `json:"audio_url_list,omitempty"`
}

func CreateRequest(
	input *dto.ModelArkVideoCreateRequest,
	upstreamModel string,
	capability model.VideoSKUCapability,
) ([]byte, error) {
	if input == nil {
		return nil, fmt.Errorf("invalid JSON video request")
	}
	if err := capability.ValidateModelArkRequest(input); err != nil {
		return nil, err
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return nil, fmt.Errorf("model is required")
	}
	if capability.DefaultDuration <= 0 || len(capability.Ratios) == 0 {
		return nil, fmt.Errorf("JSON video capability has no request defaults")
	}
	duration := capability.DefaultDuration
	if input.Duration != nil {
		duration = *input.Duration
	}
	ratio := ""
	if input.Ratio != nil {
		ratio = strings.TrimSpace(*input.Ratio)
	}
	if ratio == "" {
		ratio = capability.Ratios[0]
	}

	output := omniReferenceRequest{
		Model:         upstreamModel,
		Duration:      duration,
		Ratio:         ratio,
		ReferenceMode: "text_to_video",
	}
	firstFrames, lastFrames, referenceImages := []string{}, []string{}, []string{}
	for _, item := range input.Content {
		switch strings.TrimSpace(item.Type) {
		case "text":
			text := strings.TrimSpace(stringValue(item.Text))
			if text != "" {
				if output.Prompt != "" {
					output.Prompt += "\n"
				}
				output.Prompt += text
			}
		case "image_url":
			if item.ImageURL == nil {
				return nil, fmt.Errorf("image_url.url is required")
			}
			media := strings.TrimSpace(item.ImageURL.URL)
			if err := dto.ValidateJSONVideoMediaURL(media, dto.JSONVideoMediaImage); err != nil {
				return nil, fmt.Errorf("invalid image input: %w", err)
			}
			switch strings.TrimSpace(stringValue(item.Role)) {
			case "first_frame":
				firstFrames = append(firstFrames, media)
			case "last_frame":
				lastFrames = append(lastFrames, media)
			case "reference_image":
				referenceImages = append(referenceImages, media)
			default:
				return nil, fmt.Errorf("unsupported image role %q", stringValue(item.Role))
			}
		case "audio_url":
			if item.AudioURL == nil || strings.TrimSpace(stringValue(item.Role)) != "reference_audio" {
				return nil, fmt.Errorf("audio_url requires role=reference_audio")
			}
			media := strings.TrimSpace(item.AudioURL.URL)
			if err := dto.ValidateJSONVideoMediaURL(media, dto.JSONVideoMediaAudio); err != nil {
				return nil, fmt.Errorf("invalid audio input: %w", err)
			}
			output.AudioURLList = append(output.AudioURLList, media)
		case "video_url":
			return nil, fmt.Errorf("video input is not supported")
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	switch {
	case len(referenceImages) > 0:
		output.ReferenceMode = "omni"
		output.InputImages = referenceImages
	case len(firstFrames) == 1 && len(lastFrames) == 1:
		output.ReferenceMode = "both_frames"
		output.InputImages = []string{firstFrames[0], lastFrames[0]}
	case len(firstFrames) == 1:
		output.ReferenceMode = "first_frame"
		output.InputImages = firstFrames
	}
	return common.Marshal(output)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
