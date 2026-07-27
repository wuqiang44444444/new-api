package doubao

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const modelArkIntelligentDurationBillingSeconds = 15

// BuildTaskBillingProbe derives expression inputs from the same typed payload
// sent upstream. A video input only counts when its URL is non-empty, so an
// empty placeholder cannot select a cheaper pricing tier.
func (a *TaskAdaptor) BuildTaskBillingProbe(c *gin.Context, info *common.RelayInfo) (map[string]any, error) {
	req, err := common.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}
	payload, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload for billing probe failed")
	}

	resolution := strings.ToLower(strings.TrimSpace(payload.Resolution))
	if resolution == "" {
		resolution = "720p"
	}
	switch resolution {
	case "480p", "720p", "1080p", "4k":
	default:
		return nil, fmt.Errorf("resolution must be one of 480p, 720p, 1080p, or 4k")
	}

	hasVideoInput := false
	for _, item := range payload.Content {
		if item.Type == "video_url" && item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			hasVideoInput = true
			break
		}
	}
	durationSeconds := 0
	if payload.Duration != nil {
		durationSeconds = int(*payload.Duration)
	}
	// ModelArk uses -1 for intelligent duration. Pre-consume against the
	// provider's maximum possible duration; terminal usage later settles the
	// actual charge.
	if durationSeconds == -1 {
		durationSeconds = modelArkIntelligentDurationBillingSeconds
	}
	if durationSeconds < 0 || durationSeconds > common.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("duration_seconds must be between 0 and %d", common.MaxTaskDurationSeconds)
	}
	generateAudio := false
	if payload.GenerateAudio != nil {
		generateAudio = bool(*payload.GenerateAudio)
	}
	inputMode, controlMode := relayBillingModes(payload)

	return map[string]any{
		"resolution":       resolution,
		"has_video_input":  hasVideoInput,
		"duration_seconds": durationSeconds,
		"generate_audio":   generateAudio,
		"input_mode":       inputMode,
		"control_mode":     controlMode,
	}, nil
}
