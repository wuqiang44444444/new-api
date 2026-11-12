package seedance

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelarkusage"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Display counters come from the registered wire envelope after the existing
// normalizer has accepted the observation. Billing keeps its own usage parser.
func attachModelArkDisplayUsage(profile dto.VideoUpstreamProfile, raw, normalized []byte, expectedID string) []byte {
	var result map[string]json.RawMessage
	if common.Unmarshal(normalized, &result) != nil || result == nil {
		return normalized
	}
	changed := false
	for key := range result {
		if strings.EqualFold(key, modelarkusage.ObservationKey) {
			delete(result, key)
			changed = true
		}
	}

	var status, id string
	if common.Unmarshal(result["status"], &status) == nil && status == "succeeded" &&
		common.Unmarshal(result["id"], &id) == nil && id == expectedID {
		if display := modelArkWireDisplayUsage(profile, raw, expectedID); display != nil {
			if encoded, err := common.Marshal(display); err == nil {
				result[modelarkusage.ObservationKey] = encoded
				changed = true
			}
		}
	}
	if !changed {
		return normalized
	}
	body, err := common.Marshal(result)
	if err != nil {
		return normalized
	}
	return body
}

func modelArkDisplayUsageFromObservation(body []byte) *modelarkusage.Display {
	var fields map[string]json.RawMessage
	if common.Unmarshal(body, &fields) != nil {
		return nil
	}
	var display *modelarkusage.Display
	if common.Unmarshal(fields[modelarkusage.ObservationKey], &display) != nil {
		return nil
	}
	return display
}

func modelArkWireDisplayUsage(profile dto.VideoUpstreamProfile, raw []byte, expectedID string) *modelarkusage.Display {
	var wire map[string]json.RawMessage
	if common.Unmarshal(raw, &wire) != nil || wire == nil {
		return nil
	}
	var outerUsage json.RawMessage
	switch profile {
	case "", dto.VideoUpstreamProfileOfficial, dto.VideoUpstreamProfileThirdPartyViduModelArkV3,
		dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3:
	case dto.VideoUpstreamProfileThirdPartySynlinkVideoV1:
		var task map[string]json.RawMessage
		if common.Unmarshal(wire["task"], &task) != nil || task == nil {
			return nil
		}
		wire = task
	case dto.VideoUpstreamProfileThirdPartyRelay, dto.VideoUpstreamProfileThirdPartyMoxingModelArk,
		dto.VideoUpstreamProfileThirdPartyReverseProxy:
		var data map[string]json.RawMessage
		if common.Unmarshal(wire["data"], &data) == nil && data != nil {
			// Relay and Moxing can report usage alongside the task envelope.
			if profile != dto.VideoUpstreamProfileThirdPartyReverseProxy {
				outerUsage = wire["usage"]
			}
			wire = data
		}
	default:
		return nil
	}

	var id, taskID string
	_ = common.Unmarshal(wire["id"], &id)
	_ = common.Unmarshal(wire["task_id"], &taskID)
	if expectedID == "" || (id != expectedID && taskID != expectedID) ||
		(id != "" && taskID != "" && id != taskID) {
		return nil
	}
	display := modelArkWireCounters(profile, wire["usage"])
	if outerUsage != nil {
		outer := modelArkWireCounters(profile, outerUsage)
		display.PromptTokens = matchingModelArkCounter(display.PromptTokens, outer.PromptTokens)
		display.WebSearch = matchingModelArkCounter(display.WebSearch, outer.WebSearch)
	}
	return modelarkusage.Merge(nil, &display)
}

func modelArkWireCounters(profile dto.VideoUpstreamProfile, raw json.RawMessage) modelarkusage.Display {
	var usage, tools map[string]json.RawMessage
	if common.Unmarshal(raw, &usage) != nil {
		return modelarkusage.Display{}
	}
	prompt := modelarkusage.RawCounter(usage["prompt_tokens"])
	if profile == dto.VideoUpstreamProfileThirdPartyViduModelArkV3 {
		if value, ok := viduInteger(usage["prompt_tokens"], 0, math.MaxInt32); ok {
			prompt = common.GetPointer(int(value))
		}
	}
	_ = common.Unmarshal(usage["tool_usage"], &tools)
	return modelarkusage.Display{
		PromptTokens: prompt,
		WebSearch:    modelarkusage.RawCounter(tools["web_search"]),
	}
}

func matchingModelArkCounter(inner, outer *int) *int {
	if inner == nil {
		return outer
	}
	if outer != nil && *inner != *outer {
		return nil
	}
	return inner
}
