package seedance

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Validate the converted identity and all billing multipliers before hold. The
// plugin owns wire conversion; it cannot change the request priced by the host.
// payload is the typed northbound request (TokenSave only): the southbound
// input_mode/control_mode must match the host billing classification derived
// from the same content, so a mode drift cannot silently re-price the task.
func decodeMediaPluginCreate(result any, request map[string]any, model string, protocol dto.VideoUpstreamProtocol, spec providerModelSpec, payload *requestPayload) (*seedancePluginCreateConversion, error) {
	object, ok := result.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid media plugin conversion")
	}
	body, ok := object["body"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid media plugin body")
	}
	probe, ok := object["probe"].(map[string]any)
	if !ok || len(probe) != 0 {
		return nil, fmt.Errorf("media plugin cannot replace host billing inputs")
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	var wire map[string]any
	if err = common.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire["model"] != model {
		return nil, fmt.Errorf("media plugin model differs from the resolved provider model")
	}
	if protocol != dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 {
		expected := make(map[string]any, len(request))
		for key, value := range request {
			expected[key] = value
		}
		if expected["generate_audio"] == nil {
			expected["generate_audio"] = spec.defaultGenerateAudio
		}
		if expected["duration"] == nil && expected["frames"] == nil {
			expected["duration"] = spec.defaultDuration
		}
		if protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 || protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			if expected["resolution"] == nil || expected["resolution"] == "" {
				expected["resolution"] = "720p"
			}
			if protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
				expected["real_person_mode"] = true
				if content, ok := expected["content"].([]any); ok {
					for _, raw := range content {
						if item, ok := raw.(map[string]any); ok && item["type"] == "video_url" {
							expected["omni_reference_task_type"] = "reference"
						}
					}
				}
			}
		}
		if err = validateOfficialPluginBillingRequest(wire, expected, model); err != nil {
			return nil, err
		}
	} else {
		for _, field := range [][2]string{{"duration", "duration_seconds"}, {"generate_audio", "with_audio"}, {"resolution", "resolution"}} {
			if !reflect.DeepEqual(request[field[0]], wire[field[1]]) {
				return nil, fmt.Errorf("media plugin body differs from the request priced by the host")
			}
		}
		hasVideo := false
		if content, ok := request["content"].([]any); ok {
			for _, raw := range content {
				if item, ok := raw.(map[string]any); ok && item["type"] == "video_url" {
					if media, ok := item["video_url"].(map[string]any); ok {
						if value, ok := media["url"].(string); ok && strings.TrimSpace(value) != "" {
							hasVideo = true
						}
					}
				}
			}
		}
		videos, _ := wire["reference_videos"].([]any)
		if hasVideo != (len(videos) > 0) {
			return nil, fmt.Errorf("media plugin changed the priced video input")
		}
		if payload == nil {
			return nil, fmt.Errorf("media plugin conversion requires the priced request contract")
		}
		expectedInputMode, expectedControlMode := tokenSaveBillingModes(payload)
		if wire["input_mode"] != expectedInputMode || wire["control_mode"] != expectedControlMode {
			return nil, fmt.Errorf("media plugin body differs from the priced billing modes")
		}
	}
	return &seedancePluginCreateConversion{body: data, probe: map[string]any{}}, nil
}
