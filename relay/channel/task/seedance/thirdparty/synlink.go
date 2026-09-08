package thirdparty

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// SynlinkCreateResponse accepts only the documented task envelope. A malformed
// success cannot release the durable attempt or establish a trusted Task.
func SynlinkCreateResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Synlink create response"}
	}
	task := mapValue(root["task"])
	id, err := validateRelayTaskID(firstString(task, "id"))
	if err != nil || root["error"] != nil || task["error"] != nil || firstString(task, "status") != "pending" {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "untrusted Synlink create response"}
	}
	return common.Marshal(map[string]any{"id": id})
}

func SynlinkTaskResponse(body []byte, expectedTaskID string) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Synlink task response"}
	}
	task := mapValue(root["task"])
	id, err := validateRelayTaskID(firstString(task, "id"))
	if err != nil || id != expectedTaskID || root["error"] != nil || task["error"] != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "untrusted Synlink task identity"}
	}
	result := map[string]any{"id": id}
	// Optional metadata cannot invalidate a trusted terminal result. Normalize
	// the documented seconds and mixed timestamp formats without unsafe casts.
	if duration, ok := task["duration_seconds"].(float64); ok && duration >= 1 && duration <= relaycommon.MaxTaskDurationSeconds && math.Trunc(duration) == duration {
		result["duration"] = duration
	}
	for source, target := range map[string]string{"created_at": "created_at", "completed_at": "updated_at"} {
		switch value := task[source].(type) {
		case float64:
			if value >= 0 && value <= 253402300799 && math.Trunc(value) == value {
				result[target] = value
			}
		case string:
			if timestamp, err := time.Parse(time.RFC3339Nano, value); err == nil && timestamp.Unix() >= 0 {
				result[target] = timestamp.Unix()
			}
		}
	}
	switch firstString(task, "status") {
	case "pending":
		result["status"] = "queued"
	case "completed":
		outputs, ok := task["outputs"].([]any)
		if !ok || len(outputs) != 1 {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "Synlink completion requires one video output"}
		}
		output, ok := outputs[0].(string)
		if !ok {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Synlink video output"}
		}
		videoURL, err := relaycommon.ValidateHTTPSVideoResultURL(output)
		if err != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid Synlink video URL"}
		}
		result["status"] = "succeeded"
		result["content"] = map[string]any{"video_url": videoURL}
		terminal := normalizeTerminalTokenUsage(map[string]any{"task": map[string]any{"usage": task["usage"]}})
		if terminal.Usage != nil {
			result["usage"], result["usage_source"] = terminal.Usage, terminal.Source
		}
		if len(terminal.Evidence) > 0 {
			result["usage_evidence"] = terminal.Evidence
		}
	default:
		// The supplied guide proves pending/completed only. An undocumented
		// observation must never invent failure, cancellation or a refund.
		return nil, &relaycommon.UpstreamContractViolation{Reason: "unverified Synlink task status"}
	}
	return common.Marshal(result)
}
