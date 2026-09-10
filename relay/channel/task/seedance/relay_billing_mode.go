package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// relayBillingModes extracts the pricing dimensions (input_mode/control_mode)
// from the same typed payload the Seedance adaptor sends upstream. All media
// facts come from one pass over the typed content: a video or audio reference
// is never recorded as text-only just because no image is present.
func relayBillingModes(payload *requestPayload) (string, string) {
	hasFirstFrame := false
	hasLastFrame := false
	hasReferenceImage := false
	hasReferenceVideo := false
	hasReferenceAudio := false
	for _, item := range payload.Content {
		switch item.Type {
		case "image_url":
			if item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(item.Role)) {
			case "last_frame":
				hasLastFrame = true
			case "reference_image":
				hasReferenceImage = true
			default:
				hasFirstFrame = true
			}
		case "video_url":
			if item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
				hasReferenceVideo = true
			}
		case "audio_url":
			if item.AudioURL != nil && strings.TrimSpace(item.AudioURL.URL) != "" {
				hasReferenceAudio = true
			}
		}
	}
	switch {
	case hasReferenceVideo || hasReferenceAudio:
		return "multi_modal", "reference"
	case hasReferenceImage:
		return "multi_image", "reference"
	case hasLastFrame:
		return "multi_image", "end_frame"
	case hasFirstFrame:
		return "single_image", "none"
	default:
		return "text", "none"
	}
}

// modelArkTaskAction records the customer-visible generation mode from the
// same typed payload used for the Seedance request and billing probe.
// Reference video/audio input reuses the existing reference-to-video action.
func modelArkTaskAction(payload *requestPayload) string {
	inputMode, controlMode := relayBillingModes(payload)
	switch controlMode {
	case "reference":
		return constant.TaskActionReferenceToVideo
	case "end_frame":
		return constant.TaskActionFirstTailToVideo
	}
	if inputMode != "text" {
		return constant.TaskActionImageToVideo
	}
	for _, item := range payload.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			return constant.TaskActionTextToVideo
		}
	}
	return constant.TaskActionImageToVideo
}
