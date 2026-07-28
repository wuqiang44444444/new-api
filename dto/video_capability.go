package dto

import (
	"fmt"
	"strings"
)

func ModelArkVideoProfileIncompatibility(request *ModelArkVideoCreateRequest, profile VideoUpstreamProfile, allowServiceTier bool) string {
	if request == nil {
		return "Seedance request is unavailable"
	}
	if request.ServiceTier != nil && strings.TrimSpace(*request.ServiceTier) == "flex" && !allowServiceTier {
		return "service_tier \"flex\" is not supported by the selected video channel"
	}
	if profile != VideoUpstreamProfileThirdPartyRelay {
		return ""
	}
	hasLastFrame := false
	hasReferenceImage := false
	for _, item := range request.Content {
		itemType := strings.ToLower(strings.TrimSpace(item.Type))
		if itemType == "video_url" || itemType == "audio_url" {
			return fmt.Sprintf("%s content is not supported by the selected video channel", item.Type)
		}
		if itemType == "image_url" {
			role := ""
			if item.Role != nil {
				role = *item.Role
			}
			switch strings.ToLower(strings.TrimSpace(role)) {
			case "last_frame":
				hasLastFrame = true
			case "reference_image":
				hasReferenceImage = true
			}
		}
	}
	if hasLastFrame && hasReferenceImage {
		return "end-frame and reference-image controls cannot be combined by the selected video channel"
	}
	switch {
	case request.Seed != nil:
		return "seed is not supported by the selected video channel"
	case request.CameraFixed != nil:
		return "camera_fixed is not supported by the selected video channel"
	case request.Watermark != nil:
		return "watermark is not supported by the selected video channel"
	case request.ReturnLastFrame != nil:
		return "return_last_frame is not supported by the selected video channel"
	case request.ExecutionExpiresAfter != nil:
		return "execution_expires_after is not supported by the selected video channel"
	case request.Draft != nil:
		return "draft is not supported by the selected video channel"
	case len(request.Tools) > 0:
		return "tools is not supported by the selected video channel"
	case request.SafetyIdentifier != nil && strings.TrimSpace(*request.SafetyIdentifier) != "":
		return "safety_identifier is not supported by the selected video channel"
	case request.Priority != nil:
		return "priority is not supported by the selected video channel"
	case request.Frames != nil:
		return "frames is not supported by the selected video channel"
	}
	return ""
}
