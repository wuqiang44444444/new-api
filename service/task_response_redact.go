package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func redactTaskResponseForLog(body []byte) []byte {
	var value any
	if err := common.Unmarshal(body, &value); err != nil {
		return []byte("[unparseable task response]")
	}
	redactTaskResponseValue(value)
	redacted, err := common.Marshal(value)
	if err != nil {
		return []byte("[unavailable task response]")
	}
	return redacted
}

func redactTaskResponseValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(key)
			if normalized == "url" || strings.HasSuffix(normalized, "_url") || normalized == "authorization" || normalized == "cookie" || normalized == "api_key" || normalized == "access_token" || strings.Contains(normalized, "base64") {
				typed[key] = "[redacted]"
				continue
			}
			redactTaskResponseValue(child)
		}
	case []any:
		for _, child := range typed {
			redactTaskResponseValue(child)
		}
	}
}
