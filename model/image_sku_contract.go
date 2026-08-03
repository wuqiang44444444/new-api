package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func (capability ImageSKUCapability) ValidateRequest(request *dto.ImageRequest) error {
	if request == nil {
		return errors.New("image request is required")
	}
	if strings.TrimSpace(request.Model) != capability.PublicModel {
		return errors.New("image model does not match the published SKU capability")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return errors.New("prompt is required")
	}
	if capability.MaxPromptRunes > 0 && len([]rune(prompt)) > capability.MaxPromptRunes {
		return fmt.Errorf("prompt must not exceed %d characters", capability.MaxPromptRunes)
	}
	if !slices.Contains(capability.Sizes, strings.TrimSpace(request.Size)) {
		switch len(capability.Sizes) {
		case 1:
			return fmt.Errorf("size must be %s", capability.Sizes[0])
		case 2:
			return fmt.Errorf("size must be %s or %s", capability.Sizes[0], capability.Sizes[1])
		default:
			return fmt.Errorf("size must be one of %s or %s", strings.Join(capability.Sizes[:len(capability.Sizes)-1], ", "), capability.Sizes[len(capability.Sizes)-1])
		}
	}
	if request.N != nil && (*request.N == 0 || *request.N > capability.MaxOutputImages) {
		return fmt.Errorf("n must be between 1 and %d", capability.MaxOutputImages)
	}
	if responseFormat := strings.TrimSpace(request.ResponseFormat); responseFormat != "" && responseFormat != "url" {
		return errors.New("response_format must be url")
	}
	if request.Stream != nil && *request.Stream && !capability.SupportsStream {
		return errors.New("streaming image responses are not supported by this image model")
	}
	if field := capability.unsupportedRequestField(request); field != "" {
		return fmt.Errorf("%s is not supported by this image model", field)
	}
	if err := capability.validateExtraFields(request.Extra); err != nil {
		return err
	}
	return capability.validateInputImages(request.Image)
}

func (capability ImageSKUCapability) unsupportedRequestField(request *dto.ImageRequest) string {
	present := []struct {
		name string
		set  bool
	}{
		{"quality", strings.TrimSpace(request.Quality) != ""},
		{"style", len(request.Style) > 0},
		{"user", len(request.User) > 0},
		{"extra_fields", len(request.ExtraFields) > 0},
		{"background", len(request.Background) > 0},
		{"moderation", len(request.Moderation) > 0},
		{"output_format", len(request.OutputFormat) > 0},
		{"output_compression", len(request.OutputCompression) > 0},
		{"partial_images", len(request.PartialImages) > 0},
		{"images", len(request.Images) > 0},
		{"mask", len(request.Mask) > 0},
		{"input_fidelity", len(request.InputFidelity) > 0},
		{"watermark", request.Watermark != nil},
		{"watermark_enabled", len(request.WatermarkEnabled) > 0},
		{"user_id", len(request.UserId) > 0},
	}
	for _, field := range present {
		if field.set && !slices.Contains(capability.RequestFields, field.name) {
			return field.name
		}
	}
	return ""
}

func (capability ImageSKUCapability) validateExtraFields(fields map[string]json.RawMessage) error {
	for name, raw := range fields {
		if !slices.Contains(capability.RequestFields, name) {
			return fmt.Errorf("unsupported image field %q", name)
		}
		if name != "aspect_ratio" {
			continue
		}
		var aspectRatio string
		if err := common.Unmarshal(raw, &aspectRatio); err != nil || !slices.Contains(capability.AspectRatios, strings.TrimSpace(aspectRatio)) {
			return errors.New("aspect_ratio is not supported")
		}
	}
	return nil
}

func (capability ImageSKUCapability) validateInputImages(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var images []string
	var single string
	if err := common.Unmarshal(raw, &single); err == nil {
		images = []string{single}
	} else if err := common.Unmarshal(raw, &images); err != nil {
		return errors.New("image must be an HTTP(S) URL or an array of HTTP(S) URLs")
	}
	if len(images) > capability.MaxInputImages {
		return fmt.Errorf("image must not contain more than %d images", capability.MaxInputImages)
	}
	for _, image := range images {
		parsed, err := url.Parse(strings.TrimSpace(image))
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return errors.New("image must contain only HTTP(S) URLs")
		}
	}
	return nil
}
