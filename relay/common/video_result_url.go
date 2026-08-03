package common

import (
	"fmt"
	"net/url"
	"strings"
)

const maxSameOriginVideoResultURLLength = 8192

// ValidateSameOriginHTTPSVideoResultURL validates a provider result URL before
// it becomes a frozen private task fact or a proxy target. Signed query strings
// are preserved, while credentials and cross-origin destinations are rejected.
func ValidateSameOriginHTTPSVideoResultURL(rawResultURL, frozenBaseURL string) (string, error) {
	resultURL := strings.TrimSpace(rawResultURL)
	if resultURL == "" || len(resultURL) > maxSameOriginVideoResultURLLength {
		return "", fmt.Errorf("video result url is missing or too long")
	}
	result, err := url.Parse(resultURL)
	if err != nil || !result.IsAbs() || result.Host == "" || result.Opaque != "" {
		return "", fmt.Errorf("video result url is invalid")
	}
	if !strings.EqualFold(result.Scheme, "https") || result.User != nil {
		return "", fmt.Errorf("video result url must use https without userinfo")
	}

	base, err := url.Parse(strings.TrimSpace(frozenBaseURL))
	if err != nil || !base.IsAbs() || base.Host == "" || base.Opaque != "" ||
		!strings.EqualFold(base.Scheme, "https") || base.User != nil {
		return "", fmt.Errorf("frozen video base url is invalid")
	}
	if !strings.EqualFold(result.Hostname(), base.Hostname()) ||
		normalizedHTTPSPort(result) != normalizedHTTPSPort(base) {
		return "", fmt.Errorf("video result url is cross origin")
	}
	return resultURL, nil
}

func normalizedHTTPSPort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	return "443"
}
