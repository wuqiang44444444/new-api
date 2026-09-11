// Package thirdparty implements code-backed Seedance transport protocols.
package thirdparty

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// RelayCreateResponse 归一化第三方中转创建响应的 task_id 到 DoubaoVideo adaptor 内部 {"id": ...} 合同。
func RelayCreateResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	taskID := firstString(data, "task_id", "id")
	if taskID == "" {
		taskID = firstString(root, "task_id", "id")
	}
	taskID, err = validateRelayTaskID(taskID)
	if err != nil {
		return nil, err
	}
	return common.Marshal(map[string]any{"id": taskID})
}

// RelayTaskResponse 归一化第三方中转任务状态与结果字段到现有 DoubaoVideo 轮询合同。
// 成功终态中的明确 usage/Token 数值始终进入用量归一，不设置 Provider 或模型信任开关。
func RelayTaskResponse(body []byte, expectedTaskID string) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid JSON task response"}
	}
	data := unwrapData(root)
	taskID, err := validateRelayTaskID(firstString(data, "task_id", "id"))
	if err != nil || taskID != strings.TrimSpace(expectedTaskID) {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "task id mismatch"}
	}
	status, err := normalizeRelayStatus(firstString(data, "status", "state"))
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":     taskID,
		"status": status,
	}
	if model := firstString(data, "model"); model != "" {
		result["model"] = model
	}

	videoURL := relayResultURL(data["result"])
	if status == "succeeded" && videoURL == "" {
		return nil, &relaycommon.UpstreamContractViolation{Reason: "succeeded task has no result URL"}
	}
	if videoURL != "" {
		videoURL, err = relaycommon.ValidateHTTPSVideoResultURL(videoURL)
		if err != nil {
			return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid video result URL"}
		}
		result["content"] = map[string]any{"video_url": videoURL}
	}
	if status == "succeeded" {
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
		message := firstString(data, "error_message")
		if message == "" {
			message = findString(data, []string{"error", "message"}, []string{"message"})
		}
		if message == "" {
			message = "upstream task failed"
		}
		result["error"] = map[string]any{"message": sanitizeMessage(message)}
	}
	return common.Marshal(result)
}

// RelayTaskResponseV1 preserves the frozen legacy relay response contract for
// already-created tasks. New Moxing/TokenSave tasks must use the v2 adapter.
func RelayTaskResponseV1(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	status, err := normalizeRelayStatus(firstString(data, "status", "state"))
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"id":     firstString(data, "task_id", "id"),
		"status": status,
	}
	if model := firstString(data, "model"); model != "" {
		result["model"] = model
	}
	mediaResult := mapValue(data["result"])
	videoURL := firstString(mediaResult, "primary_url")
	if videoURL == "" {
		if urls, ok := mediaResult["urls"].([]any); ok {
			for _, value := range urls {
				if candidate, ok := value.(string); ok && strings.TrimSpace(candidate) != "" {
					videoURL = candidate
					break
				}
			}
		}
	}
	if status == "succeeded" && videoURL == "" {
		return nil, fmt.Errorf("upstream succeeded response has no result URL")
	}
	if videoURL != "" {
		result["content"] = map[string]any{"video_url": videoURL}
	}
	if usage := mapValue(data["usage"]); usage != nil {
		actual := map[string]any{}
		for _, field := range []string{"completion_tokens", "total_tokens"} {
			if value, exists := usage[field]; exists {
				actual[field] = value
			}
		}
		if len(actual) > 0 {
			result["usage"] = actual
		}
	}
	if status == "failed" {
		message := firstString(data, "error_message")
		if message == "" {
			message = findString(data, []string{"error", "message"}, []string{"message"})
		}
		if message == "" {
			message = "upstream task failed"
		}
		result["error"] = map[string]any{"message": sanitizeMessage(message)}
	}
	return common.Marshal(result)
}

func relayResultURL(value any) string {
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if validated, err := relaycommon.ValidateHTTPSVideoResultURL(text); err == nil {
			return validated
		}
		var decoded map[string]any
		if common.UnmarshalJsonStr(text, &decoded) != nil {
			return ""
		}
		value = decoded
	}
	mediaResult := mapValue(value)
	for _, field := range []string{"primary_url", "url"} {
		if candidate := firstString(mediaResult, field); candidate != "" {
			if validated, err := relaycommon.ValidateHTTPSVideoResultURL(candidate); err == nil {
				return validated
			}
		}
	}
	if urls, ok := mediaResult["urls"].([]any); ok {
		for _, value := range urls {
			candidate, ok := value.(string)
			if !ok {
				continue
			}
			if validated, err := relaycommon.ValidateHTTPSVideoResultURL(candidate); err == nil {
				return validated
			}
		}
	}
	return ""
}

// normalizeRelayStatus 归一化中转四态状态，未识别状态报错（方案 §3.2 不静默降级）。
func normalizeRelayStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "submitted":
		return "queued", nil
	case "running", "processing":
		return "running", nil
	case "succeeded", "success", "completed":
		return "succeeded", nil
	case "failed", "failure", "error", "cancelled", "canceled":
		return "failed", nil
	default:
		return "", &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
	}
}

func validateRelayTaskID(taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("upstream response has no task id")
	}
	if len(taskID) > 191 || strings.ContainsFunc(taskID, unicode.IsControl) {
		return "", fmt.Errorf("upstream response has an invalid task id")
	}
	return taskID, nil
}
