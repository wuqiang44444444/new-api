package dto

import (
	"fmt"
	"strings"
)

// ModelArkVideoMediaArraysIncompatibility validates only transport-specific
// media representation before a provider channel is selected. Provider model
// values, counts and roles are validated by the code-backed media-arrays adapter.
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
		case "video_url":
			if item.VideoURL == nil {
				return "video input is invalid: media URL is missing"
			}
			if err := ValidateVideoMediaArrayURL(item.VideoURL.URL, VideoMediaArrayVideo, true); err != nil {
				return fmt.Sprintf("video input is invalid: %s", err.Error())
			}
		}
	}
	return ""
}
