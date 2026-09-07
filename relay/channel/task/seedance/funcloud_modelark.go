package seedance

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func funCloudModelArkProviderSpec(model string) (providerModelSpec, bool) {
	spec, ok := dto.FunCloudModelArkSpec(model)
	if !ok {
		return providerModelSpec{}, false
	}
	return providerModelSpec{
		minDuration: spec.MinDuration, maxDuration: spec.MaxDuration,
		intelligentDuration: spec.MaxDuration, allowIntelligentDuration: spec.IntelligentDuration,
		resolutions: stringSet(spec.Resolutions...), maxImages: spec.MaxImages, maxVideos: spec.MaxVideos, maxAudios: spec.MaxAudios,
		allowVideos: true, allowAudios: true, defaultGenerateAudio: true, outputFormats: stringSet("mp4", "mov"),
	}, true
}

// Preserve the published five-second default, including for 2.5 where the
// Provider otherwise chooses intelligent duration. A reference video remains a
// reference; the Provider's automatic edit mode must not override its duration.
func buildFunCloudModelArkRequest(body *requestPayload) ([]byte, error) {
	payload := *body
	if payload.Duration == nil {
		value := dto.IntValue(5)
		payload.Duration = &value
	}
	if payload.Resolution == "" {
		payload.Resolution = "720p"
	}
	output := struct {
		*requestPayload
		OmniReferenceTaskType *string `json:"omni_reference_task_type,omitempty"`
	}{requestPayload: &payload}
	for _, item := range payload.Content {
		if item.Type == "video_url" {
			mode := "reference"
			output.OmniReferenceTaskType = &mode
			break
		}
	}
	return common.Marshal(output)
}
