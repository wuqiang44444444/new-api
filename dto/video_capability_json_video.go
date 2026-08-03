package dto

import (
	"fmt"
	"strings"
)

// ModelArkJSONVideoIncompatibility validates only transport-specific media
// representation. SKU fields, counts and reference combinations are owned by
// VideoSKUCapability and must not be duplicated here.
func ModelArkJSONVideoIncompatibility(request *ModelArkVideoCreateRequest) string {
	if request == nil {
		return "Seedance request is unavailable"
	}
	for _, item := range request.Content {
		switch strings.TrimSpace(item.Type) {
		case "image_url":
			if item.ImageURL == nil {
				return "image input is invalid: media URL is missing"
			}
			if err := ValidateJSONVideoMediaURL(item.ImageURL.URL, JSONVideoMediaImage); err != nil {
				return fmt.Sprintf("image input is invalid: %s", err.Error())
			}
		case "audio_url":
			if item.AudioURL == nil {
				return "audio input is invalid: media URL is missing"
			}
			if err := ValidateJSONVideoMediaURL(item.AudioURL.URL, JSONVideoMediaAudio); err != nil {
				return fmt.Sprintf("audio input is invalid: %s", err.Error())
			}
		}
	}
	return ""
}
