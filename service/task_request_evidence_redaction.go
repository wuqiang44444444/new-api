package service

import (
	"net/url"
	"strings"
)

const evidenceRedactedPlaceholder = "[REDACTED]"

func EvidenceMaskSignedURLs(text string) string {
	if text == "" || !strings.HasPrefix(strings.ToLower(text), "http") {
		return text
	}
	parsed, err := url.Parse(strings.TrimSpace(text))
	if err != nil {
		return evidenceRedactedPlaceholder
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = "redacted"
	}
	parsed.User = nil
	parsed.Fragment = ""

	return parsed.String()
}
