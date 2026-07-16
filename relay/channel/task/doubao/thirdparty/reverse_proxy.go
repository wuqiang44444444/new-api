package thirdparty

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// ReverseProxyCreateResponse 归一化第三方反代（Ark 兼容）创建响应到 DoubaoVideo adaptor
// 内部 {"id": ...} 合同，容忍官方直接结构与常见 data 包裹，任务 ID 接受 id 或 task_id。
func ReverseProxyCreateResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	taskID := firstString(data, "id", "task_id")
	if taskID == "" {
		taskID = firstString(root, "id", "task_id")
	}
	if taskID == "" {
		return nil, fmt.Errorf("upstream create response has no task id")
	}
	return common.Marshal(map[string]any{"id": taskID})
}

// ReverseProxyTaskResponse 归一化第三方反代（Ark 兼容）任务状态与结果字段到现有 DoubaoVideo
// 轮询合同，容忍 status/state、content.video_url、output.video_url 等已定义兼容差异。
func ReverseProxyTaskResponse(body []byte) ([]byte, error) {
	root, err := object(body)
	if err != nil {
		return nil, err
	}
	data := unwrapData(root)
	status, err := normalizeReverseProxyStatus(firstString(data, "status", "state"))
	if err != nil {
		return nil, err
	}
	videoURL := findString(data, []string{"content", "video_url"}, []string{"output", "video_url"}, []string{"video_url"})
	// 反代终态合同与中转一致：succeeded 必须带结果 URL，否则 fail closed，避免标记成功却无内容（方案 §3.3）。
	if status == "succeeded" && videoURL == "" {
		return nil, fmt.Errorf("upstream succeeded response has no result URL")
	}
	result := map[string]any{
		"id":     firstString(data, "id", "task_id"),
		"status": status,
	}
	if model := firstString(data, "model"); model != "" {
		result["model"] = model
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
	if message := findString(data, []string{"error", "message"}, []string{"message"}); message != "" {
		result["error"] = map[string]any{"message": sanitizeMessage(message)}
	}
	return common.Marshal(result)
}

// normalizeReverseProxyStatus 归一化反代（Ark 兼容）状态字段，未识别状态报错（fail closed）。
// 与中转协议 normalizeRelayStatus 对称：未知状态不得原样返回，否则 adaptor ParseTaskResult
// 会把其归入 default 分支视为 IN_PROGRESS，造成永久轮询、永不结算（方案 §3.3 / P1-A）。
func normalizeReverseProxyStatus(status string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending", "queued", "processing", "running", "succeeded", "failed":
		return strings.ToLower(strings.TrimSpace(status)), nil
	case "success", "completed":
		return "succeeded", nil
	case "failure", "error", "cancelled", "canceled":
		return "failed", nil
	default:
		return "", fmt.Errorf("upstream reverse-proxy response has unsupported status %q", status)
	}
}
