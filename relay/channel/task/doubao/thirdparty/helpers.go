package thirdparty

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// object 解码上游响应根对象。
func object(body []byte) (map[string]any, error) {
	var root map[string]any
	if err := common.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode upstream response: %w", err)
	}
	return root, nil
}

// unwrapData 返回 data 包裹层，缺失时回退到根对象，容忍常见的 data 包裹差异。
func unwrapData(root map[string]any) map[string]any {
	if data := mapValue(root["data"]); data != nil {
		return data
	}
	return root
}

// sanitizeMessage 屏蔽上游错误信息中的敏感片段并截断过长内容，避免凭证泄露到客户端。
func sanitizeMessage(message string) string {
	normalized := strings.ToLower(message)
	for _, sensitive := range []string{"http://", "https://", "bearer ", "api_key", "api-key", "cookie", "authorization"} {
		if strings.Contains(normalized, sensitive) {
			return "upstream request failed"
		}
	}
	if len(message) > 512 {
		return message[:512]
	}
	return message
}

// firstString 返回对象中首个非空字符串字段（按给定 key 顺序）。
func firstString(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// findString 按多组路径查找首个非空字符串字段，用于容忍嵌套兼容差异。
func findString(object map[string]any, paths ...[]string) string {
	for _, path := range paths {
		var current any = object
		for _, part := range path {
			next, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = next[part]
		}
		if value, ok := current.(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}
