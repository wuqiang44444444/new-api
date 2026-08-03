package relay

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

type taskUpstreamHTTPError struct {
	statusCode      int
	providerCode    string
	providerMessage string
}

func (e *taskUpstreamHTTPError) Error() string {
	base := fmt.Sprintf("upstream task request returned HTTP %d", e.statusCode)
	switch {
	case e.providerCode != "" && e.providerMessage != "":
		return fmt.Sprintf("%s (%s): %s", base, e.providerCode, e.providerMessage)
	case e.providerCode != "":
		return fmt.Sprintf("%s (%s)", base, e.providerCode)
	case e.providerMessage != "":
		return base + ": " + e.providerMessage
	default:
		return base
	}
}

func parseTaskUpstreamHTTPError(statusCode int, body []byte) *taskUpstreamHTTPError {
	result := &taskUpstreamHTTPError{statusCode: statusCode}
	var root map[string]any
	if len(body) == 0 || common.Unmarshal(body, &root) != nil {
		return result
	}

	detail := root
	if nested, ok := root["error"].(map[string]any); ok {
		detail = nested
	}

	if code, ok := detail["code"].(string); ok {
		result.providerCode = safeTaskUpstreamToken(code)
	}

	if message, ok := detail["message"].(string); ok {
		message = strings.Join(strings.Fields(message), " ")
		message = service.SanitizeTaskErrorText(errors.New(message))
		if message != "task request failed" && message != "upstream task request failed" {
			result.providerMessage = message
		}
	}
	return result
}

func safeTaskUpstreamToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '.' && char != '_' && char != '-' && char != ':' {
			return ""
		}
	}
	return value
}
