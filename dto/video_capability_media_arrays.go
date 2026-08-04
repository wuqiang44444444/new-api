package dto

import (
	"fmt"
	"strings"
)

// ModelArkVideoMediaArraysIncompatibility validates only transport-specific
// media representation before a provider channel is selected. SKU values,
// counts and roles remain owned by VideoSKUCapability.
func ModelArkVideoMediaArraysIncompatibility(request *ModelArkVideoCreateRequest) string {
	if request == nil {
		return "video request is unavailable"
	}
	for _, item := range request.Content {
		switch strings.TrimSpace(item.Type) {
		case "image_url":
			if item.ImageURL == nil {
				return "image input is invalid: media URL is missing"
			}
			if err := ValidateVideoMediaArrayURL(item.ImageURL.URL, VideoMediaArrayImage, true); err != nil {
				return fmt.Sprintf("image input is invalid: %s", err.Error())
			}
		case "audio_url":
			if item.AudioURL == nil {
				return "audio input is invalid: media URL is missing"
			}
			if err := ValidateVideoMediaArrayURL(item.AudioURL.URL, VideoMediaArrayAudio, true); err != nil {
				return fmt.Sprintf("audio input is invalid: %s", err.Error())
			}
		}
	}
	return ""
}
