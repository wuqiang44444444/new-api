// Package mediaarrays implements the Seedance media-arrays protocol.
package mediaarrays

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type createRequest struct {
	Model    string   `json:"model"`
	Prompt   string   `json:"prompt"`
	Duration int      `json:"duration"`
	Size     string   `json:"size"`
	Images   []string `json:"images,omitempty"`
	Audios   []string `json:"audios,omitempty"`
	Videos   []string `json:"videos,omitempty"`
}

func CreateRequest(
	input *dto.ModelArkVideoCreateRequest,
	upstreamModel string,
) ([]byte, error) {
	if input == nil {
		return nil, fmt.Errorf("invalid JSON video media-arrays request")
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		return nil, fmt.Errorf("model is required")
	}
	if input.Duration == nil || *input.Duration <= 0 || *input.Duration > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("duration is required for this model")
	}
	duration := *input.Duration
	if input.Resolution == nil || strings.TrimSpace(*input.Resolution) == "" {
		return nil, fmt.Errorf("resolution is required for this model")
	}
	resolution := strings.ToLower(strings.TrimSpace(*input.Resolution))
	ratio := ""
	if input.Ratio != nil {
		ratio = strings.TrimSpace(*input.Ratio)
	}
	size, ok := ResolveVideoSize(upstreamModel, resolution, ratio)
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
			role := ""
			if item.Role != nil {
				role = strings.TrimSpace(*item.Role)
			}
			if item.VideoURL == nil || role != "reference_video" {
				return nil, fmt.Errorf("video_url requires role=reference_video")
			}
			media := strings.TrimSpace(item.VideoURL.URL)
			if err := dto.ValidateVideoMediaArrayURL(media, dto.VideoMediaArrayVideo, false); err != nil {
				return nil, fmt.Errorf("invalid video input: %w", err)
			}
			output.Videos = append(output.Videos, media)
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	if len(output.Images) > 9 || len(output.Audios) > 3 || len(output.Videos) > 3 {
		return nil, fmt.Errorf("media input exceeds the protocol limit")
	}
	return common.Marshal(output)
}
