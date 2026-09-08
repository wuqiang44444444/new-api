package common

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	taskErrorAuthHeader = regexp.MustCompile(`(?i)\b(?:authorization|cookie)["']?\s*[:：=]`)
	taskErrorURL        = regexp.MustCompile(`(?i)https?://[^\s<>"'，。；）;]+`)
	taskErrorCredential = regexp.MustCompile(`(?i)["']?\b(?:api[_-]?key|access[_-]?token|token|client_secret|bytedtoken)["']?\s*[:：=]\s*(?:"(?:\\.|[^"\\])*(?:"|\\?$)|'(?:\\.|[^'\\])*(?:'|\\?$)|(?:bearer\s+)?[^\s,;，；]+)|\bbearer\s+[^\s,;]+|\bsk-[a-zA-Z0-9_-]+`)
	taskErrorBody       = regexp.MustCompile(`(?i)(?:^|[;\n]\s*)body\s*[:=]\s*[\{\[]`)
	taskErrorChannel    = regexp.MustCompile(`(?i)(?:channel[ _-]*id|渠道\s*ID)\s*[:：=]?\s*\d+|(?:channel|渠道)\s*#\s*\d+`)
	taskErrorProvider   = regexp.MustCompile(`(?i)\b(?:funcloud|leonecloud|moxing|volcengine|byteplus|tokensave|feicai|openai|bytedance)\b|火山引擎|字节跳动|墨行|飞彩`)
	taskErrorCode       = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,128}$`)
)

// PublicTaskErrorMessage keeps the actionable business reason. Raw diagnostic
// bodies are never messages; URLs, credentials and routing identity are removed
// without replacing an otherwise useful copyright/parameter/moderation error.
func PublicTaskErrorMessage(message string) string {
	return PublicTaskErrorMessageForModel(message, "", "")
}

// PublicTaskErrorMessageForModel uses the identity frozen by the request/task,
// never a current channel mapping. Customer model names remain public names.
func PublicTaskErrorMessageForModel(message, originModel, upstreamModel string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	lower := strings.ToLower(message)
	if strings.HasPrefix(lower, "poll failed: not_found") {
		return "Video task could not be found by the video service."
	}
	if strings.HasPrefix(lower, "poll failed:") {
		return "Task status could not be retrieved. Please contact support with the request ID."
	}
	// Remove credentials before protecting model names: a public alias such as
	// "token" must never hide the key of a credential assignment from the filter.
	message = sanitizeTaskDiagnostic(message)
	const customerModel = "\x00customer-model\x00"
	var identities []string
	if upstreamModel != "" && upstreamModel != originModel {
		identities = append(identities, upstreamModel, "requested model")
	}
	if originModel != "" {
		// Match the longer name first when one identity is a prefix of the other.
		if len(originModel) > len(upstreamModel) {
			identities = append([]string{originModel, customerModel}, identities...)
		} else {
			identities = append(identities, originModel, customerModel)
		}
	}
	if len(identities) > 0 {
		message = strings.NewReplacer(identities...).Replace(message)
	}
	message = taskErrorProvider.ReplaceAllString(message, "video service")
	message = strings.ReplaceAll(message, customerModel, originModel)
	if message == "[URL]" || message == "[redacted]" {
		return "Video generation failed"
	}
	return truncateTaskErrorMessage(message)
}

// SanitizeTaskDiagnostic keeps technical explanations for operator logs. It
// removes access details but does not turn parser/transport errors into user copy.
func SanitizeTaskDiagnostic(message string) string {
	return truncateTaskErrorMessage(sanitizeTaskDiagnostic(message))
}

// Keep truncation at the public/log boundary so it cannot split a credential or
// upstream model identity before redaction has inspected the complete message.
func sanitizeTaskDiagnostic(message string) string {
	message = strings.TrimSpace(message)
	// Authentication headers can contain several space/semicolon-separated secrets.
	// Do not attempt to retain parts of a diagnostic that includes them.
	if taskErrorAuthHeader.MatchString(message) {
		return "Video service request failed"
	}
	lower := strings.ToLower(message)
	if strings.HasPrefix(message, "{") || strings.HasPrefix(message, "[{") || strings.HasPrefix(message, "[\"") || strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype") {
		return "Video service returned an unreadable response"
	}
	if loc := taskErrorBody.FindStringIndex(message); loc != nil {
		message = strings.TrimSpace(message[:loc[0]])
		if message == "" {
			return "Video service returned an unreadable response"
		}
	}
	message = taskErrorURL.ReplaceAllString(message, "[URL]")
	message = taskErrorCredential.ReplaceAllString(message, "[redacted]")
	message = taskErrorChannel.ReplaceAllString(message, "")
	message = strings.TrimSpace(strings.ToValidUTF8(MaskSensitiveInfo(message), ""))
	return message
}

func truncateTaskErrorMessage(message string) string {
	if len(message) > 512 {
		message = message[:512]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		message += "…"
	}
	return message
}

// PublicTaskErrorCode accepts only a short code, never arbitrary provider text.
func PublicTaskErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if !taskErrorCode.MatchString(code) || taskErrorProvider.MatchString(code) {
		return ""
	}
	// These are host operation failures, not provider business codes.
	switch code {
	case "fail_to_fetch_task", "task_request_failed", "do_request_failed", "build_request_failed",
		"plugin_submit_response_failed", "plugin_submit_response_invalid", "read_response_body_failed",
		"copy_response_body_failed", "get_task_failed", "convert_to_openai_video_failed", "marshal_response_failed":
		return ""
	}
	return code
}
