package seedance

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/mediaarrays"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const modelArkIntelligentDurationBillingSeconds = 15

// BuildTaskBillingProbe derives expression inputs from the same typed payload
// sent upstream. A video input only counts when its URL is non-empty, so an
// empty placeholder cannot select a cheaper pricing tier.
func (a *TaskAdaptor) BuildTaskBillingProbe(c *gin.Context, info *common.RelayInfo) (map[string]any, error) {
	if info == nil {
		return nil, fmt.Errorf("relay info is unavailable")
	}
	payload, typed, err := a.modelArkContractPayload(c)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload for billing probe failed")
	}
	if !typed {
		return nil, fmt.Errorf("Seedance billing requires the ModelArk V3 request contract")
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
	durationSeconds := 5
	if payload.Duration != nil {
		durationSeconds = int(*payload.Duration)
	}
	if payload.Frames != nil {
		durationSeconds = (int(*payload.Frames) + 23) / 24
	}
	// ModelArk uses -1 for intelligent duration. Pre-consume against the
	// provider's maximum possible duration. Only an implementation with verified
	// terminal usage may later settle below this frozen upper bound.
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

	probe := map[string]any{
		"resolution":       resolution,
		"has_video_input":  hasVideoInput,
		"duration_seconds": durationSeconds,
		"generate_audio":   generateAudio,
		"input_mode":       inputMode,
		"control_mode":     controlMode,
	}
	if a.profile == dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays {
		if info.ChannelMeta == nil {
			return nil, fmt.Errorf("JSON video media-arrays billing capability is unavailable")
		}
		upstreamModel := strings.TrimSpace(info.UpstreamModelName)
		if upstreamModel == "" {
			upstreamModel = strings.TrimSpace(payload.Model)
		}
		resolution = strings.ToLower(strings.TrimSpace(payload.Resolution))
		ratio := strings.TrimSpace(payload.Ratio)
		size, ok := mediaarrays.ResolveVideoSize(
			upstreamModel,
			resolution,
			ratio,
		)
		if !ok {
			return nil, fmt.Errorf("resolution %q and ratio %q have no verified provider size", resolution, ratio)
		}
		probe["resolution"] = resolution
		probe["ratio"] = ratio
		probe["size"] = size.Value
		probe["size_multiplier"] = size.Multiplier
		probe["billing_size_class"] = size.BillingClass
		probe["billing_mode"] = "per-second"
	}
	return probe, nil
}
