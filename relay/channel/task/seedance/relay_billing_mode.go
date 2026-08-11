package seedance

import "strings"

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
