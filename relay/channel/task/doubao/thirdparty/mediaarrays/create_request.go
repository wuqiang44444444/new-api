package mediaarrays

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

type createRequest struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Size     string   `json:"size"`
	Images   []string `json:"images,omitempty"`
	Audios   []string `json:"audios,omitempty"`
}

func CreateRequest(input *dto.ModelArkVideoCreateRequest, upstreamModel string, capability model.VideoSKUCapability) ([]byte, error) {
	if input == nil {
		return nil, fmt.Errorf("invalid JSON video media-arrays request")
	}
	if err := capability.ValidateModelArkRequest(input); err != nil {
		return nil, err
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return nil, fmt.Errorf("model is required")
	}
	if capability.DefaultDuration <= 0 || len(capability.Ratios) == 0 {
		return nil, fmt.Errorf("JSON video media-arrays capability has no request defaults")
	}
	duration := capability.DefaultDuration
	if input.Duration != nil {
		duration = *input.Duration
	}
	resolution := capability.Resolution
	if input.Resolution != nil {
		resolution = strings.ToLower(strings.TrimSpace(*input.Resolution))
	}
	ratio := capability.Ratios[0]
	if input.Ratio != nil && strings.TrimSpace(*input.Ratio) != "" {
		ratio = strings.TrimSpace(*input.Ratio)
	}
	size, ok := ResolveVideoSize(resolution, ratio)
	if !ok {
		return nil, fmt.Errorf("resolution %q and ratio %q have no verified provider size", resolution, ratio)
	}
	output := createRequest{Model: upstreamModel, Duration: duration, Size: size.Value}
	for _, item := range input.Content {
		switch strings.TrimSpace(item.Type) {
		case "text":
			text := ""
			if item.Text != nil {
				text = strings.TrimSpace(*item.Text)
			}
			if text != "" {
				if output.Prompt != "" {
					output.Prompt += "\n"
				}
				output.Prompt += text
			}
		case "image_url":
			role := ""
			if item.Role != nil {
				role = strings.TrimSpace(*item.Role)
			}
			if item.ImageURL == nil || role != "reference_image" {
				return nil, fmt.Errorf("image_url requires role=reference_image")
			}
			media := strings.TrimSpace(item.ImageURL.URL)
			if err := dto.ValidateVideoMediaArrayURL(media, dto.VideoMediaArrayImage, false); err != nil {
				return nil, fmt.Errorf("invalid image input: %w", err)
			}
			output.Images = append(output.Images, media)
		case "audio_url":
			role := ""
			if item.Role != nil {
				role = strings.TrimSpace(*item.Role)
			}
			if item.AudioURL == nil || role != "reference_audio" {
				return nil, fmt.Errorf("audio_url requires role=reference_audio")
			}
			media := strings.TrimSpace(item.AudioURL.URL)
			if err := dto.ValidateVideoMediaArrayURL(media, dto.VideoMediaArrayAudio, false); err != nil {
				return nil, fmt.Errorf("invalid audio input: %w", err)
			}
			output.Audios = append(output.Audios, media)
		case "video_url":
			return nil, fmt.Errorf("video input is not supported")
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	return common.Marshal(output)
}
