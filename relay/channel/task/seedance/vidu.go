package seedance

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Vidu documents integers but returns decimal strings in live responses. Only
// this registered protocol accepts both spellings; official Ark remains strict.
// Parsing raw bytes also keeps exponent/fraction spellings out of billing.
func viduInteger(raw json.RawMessage, minimum, maximum int64) (int64, bool) {
	value := string(raw)
	if len(value) > 0 && value[0] == '"' {
		if common.Unmarshal(raw, &value) != nil {
			return 0, false
		}
	}
	digits := value
	if minimum < 0 && strings.HasPrefix(digits, "-") {
		digits = digits[1:]
	}
	if len(digits) == 0 || len(digits) > 19 || (len(digits) > 1 && digits[0] == '0') {
		return 0, false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	number, err := strconv.ParseInt(value, 10, 64)
	return number, err == nil && number >= minimum && number <= maximum
}

// normalizeViduTaskResponse owns identity, field and usage evidence before the
// frozen plugin sees the response. The plugin cannot replace these money facts.
func normalizeViduTaskResponse(body []byte, expectedID string) ([]byte, error) {
	var raw map[string]json.RawMessage
	if common.Unmarshal(body, &raw) != nil || raw == nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu task response"}
	}
	var id, status string
	if common.Unmarshal(raw["id"], &id) != nil || id == "" || id != expectedID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "Vidu task id mismatch"}
	}
	if common.Unmarshal(raw["status"], &status) != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu task status"}
	}
	switch status {
	case "queued", "running", "succeeded", "failed", "expired":
	default:
		return nil, &relaycommon.UpstreamContractViolation{Reason: "unknown Vidu task status"}
	}
	result := map[string]any{"id": id, "status": status}
	for _, field := range []string{"model", "resolution", "ratio", "service_tier"} {
		if value, exists := raw[field]; exists && string(value) != "null" {
			var text string
			if common.Unmarshal(value, &text) != nil {
				return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu " + field}
			}
			result[field] = text
		}
	}
	for _, field := range []struct {
		wire, canonical string
		min, max        int64
	}{
		{"seed", "seed", -1, math.MaxInt32},
		{"duration", "duration", 0, 60},
		{"created_at", "created_at", 0, math.MaxInt64},
		{"updated_at", "updated_at", 0, math.MaxInt64},
		{"frames_per_second", "framespersecond", 0, 120},
		{"execution_expires_after", "execution_expires_after", 0, 259200},
	} {
		if value, exists := raw[field.wire]; exists && string(value) != "null" {
			number, ok := viduInteger(value, field.min, field.max)
			if !ok {
				return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu " + field.wire}
			}
			result[field.canonical] = number
		}
	}
	for _, field := range []string{"generate_audio", "draft"} {
		if value, exists := raw[field]; exists && string(value) != "null" {
			var flag bool
			if common.Unmarshal(value, &flag) != nil {
				return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu " + field}
			}
			result[field] = flag
		}
	}
	var content struct {
		VideoURL     string `json:"video_url,omitempty"`
		LastFrameURL string `json:"last_frame_url,omitempty"`
	}
	if value := raw["content"]; len(value) > 0 && common.Unmarshal(value, &content) != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu task content"}
	}
	var failure struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	}
	if value := raw["error"]; len(value) > 0 && common.Unmarshal(value, &failure) != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu task error"}
	}
	if status == "failed" || status == "expired" {
		if content.VideoURL != "" || content.LastFrameURL != "" {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "conflicting Vidu failure content"}
		}
		result["error"] = failure
	} else if failure.Code != "" || failure.Message != "" {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "conflicting Vidu task error"}
	}
	if status != "succeeded" {
		return common.Marshal(result)
	}
	if strings.TrimSpace(content.VideoURL) == "" {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "Vidu succeeded task has no video"}
	}
	videoURL, err := relaycommon.ValidateHTTPSVideoResultURL(content.VideoURL)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu video result URL"}
	}
	content.VideoURL = videoURL
	if content.LastFrameURL != "" {
		content.LastFrameURL, err = relaycommon.ValidateHTTPSVideoResultURL(content.LastFrameURL)
		if err != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Vidu last frame URL"}
		}
	}
	result["content"] = content
	var rawUsage map[string]json.RawMessage
	_ = common.Unmarshal(raw["usage"], &rawUsage)
	usage, evidence := map[string]int64{}, map[string]int64{}
	valid := true
	for _, field := range []string{"completion_tokens", "total_tokens", "prompt_tokens"} {
		value, exists := rawUsage[field]
		if !exists || string(value) == "null" {
			continue
		}
		number, ok := viduInteger(value, 0, math.MaxInt32)
		if !ok {
			valid = false
			continue
		}
		usage[field], evidence["usage."+field] = number, number
	}
	completion, hasCompletion := usage["completion_tokens"]
	if total, exists := usage["total_tokens"]; exists && hasCompletion && total < completion {
		valid = false
	}
	if valid && hasCompletion {
		if _, exists := usage["total_tokens"]; !exists {
			usage["total_tokens"] = completion
		}
		result["usage"], result["usage_source"] = usage, "usage.completion_tokens"
	}
	if len(evidence) > 0 {
		result["usage_evidence"] = evidence
	}
	// Missing/invalid usage does not erase a valid video or invent a zero charge.
	// The existing awaiting_usage worker retries the same frozen task contract.
	return common.Marshal(result)
}
