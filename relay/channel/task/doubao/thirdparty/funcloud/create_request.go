package funcloud

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

type createContent struct {
	Type     string             `json:"type"`
	Text     *string            `json:"text,omitempty"`
	Role     *string            `json:"role,omitempty"`
	ImageURL *dto.VideoMediaURL `json:"image_url,omitempty"`
	VideoURL *dto.VideoMediaURL `json:"video_url,omitempty"`
	AudioURL *dto.VideoMediaURL `json:"audio_url,omitempty"`
}

type createRequest struct {
	Content       []createContent `json:"content"`
	Ratio         string          `json:"ratio"`
	Duration      int             `json:"duration"`
	Resolution    string          `json:"resolution"`
	GenerateAudio *bool           `json:"generateAudio,omitempty"`
	Watermark     *bool           `json:"watermark,omitempty"`
	Seed          *int            `json:"seed,omitempty"`
	CameraFixed   *bool           `json:"cameraFixed,omitempty"`
}

func CreateRequest(input *dto.ModelArkVideoCreateRequest, capability model.VideoSKUCapability) ([]byte, error) {
	if input == nil {
		return nil, fmt.Errorf("FunCloud video request is unavailable")
	}
	if err := capability.ValidateModelArkRequest(input); err != nil {
		return nil, err
	}
	if capability.DefaultDuration <= 0 || len(capability.Ratios) == 0 {
		return nil, fmt.Errorf("FunCloud video capability has no request defaults")
	}

	duration := capability.DefaultDuration
	if input.Duration != nil {
		duration = *input.Duration
	}
	ratio := capability.Ratios[0]
	if input.Ratio != nil && strings.TrimSpace(*input.Ratio) != "" {
		ratio = strings.TrimSpace(*input.Ratio)
	}
	resolution := strings.TrimSpace(capability.Resolution)
	if input.Resolution != nil && strings.TrimSpace(*input.Resolution) != "" {
		resolution = strings.TrimSpace(*input.Resolution)
	}
	if resolution == "" && slices.Contains(capability.Resolutions, "720p") {
		resolution = "720p"
	}
	if resolution == "" {
		return nil, fmt.Errorf("resolution is required by the FunCloud v2 adapter")
	}

	output := createRequest{
		Content:       make([]createContent, 0, len(input.Content)),
		Ratio:         ratio,
		Duration:      duration,
		Resolution:    resolution,
		GenerateAudio: input.GenerateAudio,
		Watermark:     input.Watermark,
		Seed:          input.Seed,
		CameraFixed:   input.CameraFixed,
	}
	for _, item := range input.Content {
		content := createContent{Type: strings.TrimSpace(item.Type), Text: item.Text, Role: item.Role}
		switch content.Type {
		case "text":
			if item.Text == nil || strings.TrimSpace(*item.Text) == "" {
				return nil, fmt.Errorf("text content is empty")
			}
		case "image_url":
			if err := validateMedia(item.ImageURL); err != nil {
				return nil, fmt.Errorf("invalid image input: %w", err)
			}
			content.ImageURL = item.ImageURL
		case "video_url":
			if err := validateMedia(item.VideoURL); err != nil {
				return nil, fmt.Errorf("invalid video input: %w", err)
			}
			content.VideoURL = item.VideoURL
		case "audio_url":
			if err := validateMedia(item.AudioURL); err != nil {
				return nil, fmt.Errorf("invalid audio input: %w", err)
			}
			content.AudioURL = item.AudioURL
		default:
			return nil, fmt.Errorf("unsupported content type %q", item.Type)
		}
		output.Content = append(output.Content, content)
	}
	return common.Marshal(output)
}

func validateMedia(media *dto.VideoMediaURL) error {
	if media == nil {
		return fmt.Errorf("media URL is missing")
	}
	raw := strings.TrimSpace(media.URL)
	if strings.HasPrefix(raw, "asset://") {
		return fmt.Errorf("platform asset references must be resolved to HTTPS before FunCloud conversion")
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("media URL must be absolute HTTPS")
	}
	return nil
}
