package relay

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

type taskUpstreamHTTPError struct {
	statusCode      int
	providerCode    string
	providerMessage string
}

// TaskErrorDetails keeps the business message separate from diagnostic prefixes.
func (e *taskUpstreamHTTPError) TaskErrorDetails() (string, string) {
	return e.providerCode, e.providerMessage
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

func parseTaskUpstreamHTTPError(statusCode int, body []byte, info *relaycommon.RelayInfo) *taskUpstreamHTTPError {
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
		originModel, upstreamModel := "", ""
		if info != nil && info.TaskRelayInfo != nil && model.IsLinkVideoTaskClientProtocol(info.TaskRelayInfo.ClientProtocol) {
			originModel, upstreamModel = info.OriginModelName, info.UpstreamModelName
		}
		message = common.PublicTaskErrorMessageForModel(message, originModel, upstreamModel)
		// Retain the native submit path's existing model replacement semantics.
		if originModel == "" && info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" && info.UpstreamModelName != info.OriginModelName {
			message = strings.ReplaceAll(message, info.UpstreamModelName, "requested model")
		}
		if message != "task request failed" && message != "upstream task request failed" {
			result.providerMessage = message
		}
	}
	return result
}

func taskUpstreamSubmissionError(err *taskUpstreamHTTPError, info *relaycommon.RelayInfo) *dto.TaskError {
	if info != nil && info.TaskRelayInfo != nil && model.IsLinkVideoTaskClientProtocol(info.TaskRelayInfo.ClientProtocol) {
		return service.TaskErrorWrapperForModel(err, "fail_to_fetch_task", err.statusCode, info.OriginModelName, info.UpstreamModelName)
	}
	return service.TaskErrorWrapper(err, "fail_to_fetch_task", err.statusCode)
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
