package thirdparty

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// FunCloudModelArkCreateResponse accepts only the documented top-level ID.
// A malformed response cannot establish a trusted Task after POST.
func FunCloudModelArkCreateResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	if root["error"] != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "create response contains an error"}
	}
	id, err := validateRelayTaskID(firstString(root, "id"))
	if err != nil {
		return nil, err
	}
	return common.Marshal(map[string]any{"id": id})
}

func FunCloudModelArkTaskResponse(body []byte, expectedTaskID string) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid JSON task response"}
	}
	id, err := validateRelayTaskID(firstString(root, "id"))
	if err != nil || id != expectedTaskID {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "task id mismatch"}
	}
	status := firstString(root, "status")
	switch status {
	case "submitted":
		status = "queued"
	case "running", "succeeded", "failed":
	default:
		return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
	}
	result := map[string]any{"id": id, "status": status}
	for _, key := range []string{"model", "created_at", "updated_at", "duration", "resolution", "ratio", "seed", "generate_audio", "frames", "framespersecond"} {
		if value, ok := root[key]; ok {
			result[key] = value
		}
	}
	if status == "succeeded" {
		videoURL, err := relaycommon.ValidateHTTPSVideoResultURL(firstString(mapValue(root["content"]), "video_url"))
		if err != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid video result URL"}
		}
		result["content"] = map[string]any{"video_url": videoURL}
		terminal := normalizeTerminalTokenUsage(root)
		if terminal.Usage != nil {
			result["usage"] = terminal.Usage
			result["usage_source"] = terminal.Source
		}
		if len(terminal.Evidence) > 0 {
			result["usage_evidence"] = terminal.Evidence
		}
	}
	if status == "failed" {
		failure := mapValue(root["error"])
		message := firstString(failure, "message")
		if message == "" {
			message = "upstream task failed"
		}
		result["error"] = map[string]any{"code": sanitizeMessage(firstString(failure, "code")), "message": sanitizeMessage(message)}
	}
	return common.Marshal(result)
}
