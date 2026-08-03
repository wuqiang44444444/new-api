package service

import (
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const maxTaskErrorMessageBytes = 512

// SanitizeTaskErrorText returns a bounded, credential-safe task error message.
func SanitizeTaskErrorText(err error) string {
	return sanitizeTaskErrorText(err)
}

func sanitizeTaskErrorText(err error) string {
	if err == nil {
		return "task request failed"
	}
	text := strings.TrimSpace(common.MaskSensitiveInfo(err.Error()))
	lower := strings.ToLower(text)
	if text == "" {
		return "task request failed"
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "authorization") ||
		strings.Contains(lower, "bytedtoken") ||
		strings.Contains(lower, "access_token") ||
		strings.Contains(lower, "client_secret") ||
		strings.Contains(lower, "response body") ||
		strings.Contains(lower, "body:") ||
		strings.Contains(lower, "http://") ||
		strings.Contains(lower, "https://") {
		return "upstream task request failed"
	}
	if len(text) <= maxTaskErrorMessageBytes {
		return text
	}
	text = text[:maxTaskErrorMessageBytes]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return strings.TrimSpace(text) + "…"
}
