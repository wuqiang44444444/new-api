package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// relayBillingModes exposes the third-party relay protocol's pricing dimensions
// (input_mode/control_mode) derived from the same typed payload the Seedance
// adaptor sends upstream.
func relayBillingModes(payload *requestPayload) (string, string) {
	hasFirstFrame := false
	hasLastFrame := false
	hasReferenceImage := false
	for _, item := range payload.Content {
		if item.Type != "image_url" || item.ImageURL == nil || strings.TrimSpace(item.ImageURL.URL) == "" {
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
	}
	switch {
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
func modelArkTaskAction(payload *requestPayload) string {
	inputMode, controlMode := relayBillingModes(payload)
	switch controlMode {
	case "reference":
		return constant.TaskActionReferenceGenerate
	case "end_frame":
		return constant.TaskActionFirstTailGenerate
	}
	if inputMode != "text" {
		return constant.TaskActionGenerate
	}

	hasText := false
	for _, item := range payload.Content {
		switch item.Type {
		case "text":
			hasText = hasText || strings.TrimSpace(item.Text) != ""
		case "video_url":
			if item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
				return constant.TaskActionGenerate
			}
		case "audio_url":
			if item.AudioURL != nil && strings.TrimSpace(item.AudioURL.URL) != "" {
				return constant.TaskActionGenerate
			}
		}
	}
	if hasText {
		return constant.TaskActionTextGenerate
	}
	return constant.TaskActionGenerate
}
