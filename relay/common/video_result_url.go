package common

import (
	"fmt"
	"net/url"
	"strings"
)

const maxSameOriginVideoResultURLLength = 8192

// ValidateSameOriginVideoResultURL validates a provider result URL before
// it becomes a frozen private task fact or a proxy target. Signed query strings
// are preserved, while credentials and cross-origin destinations are rejected.
func ValidateSameOriginVideoResultURL(rawResultURL, frozenBaseURL string) (string, error) {
	resultURL := strings.TrimSpace(rawResultURL)
	if resultURL == "" || len(resultURL) > maxSameOriginVideoResultURLLength {
		return "", fmt.Errorf("video result url is missing or too long")
	}
	result, err := url.Parse(resultURL)
	if err != nil || !result.IsAbs() || result.Host == "" || result.Opaque != "" {
		return "", fmt.Errorf("video result url is invalid")
	}
	if !isHTTPVideoURL(result) || result.User != nil {
		return "", fmt.Errorf("video result url must use http or https without userinfo")
	}

	base, err := url.Parse(strings.TrimSpace(frozenBaseURL))
	if err != nil || !base.IsAbs() || base.Host == "" || base.Opaque != "" ||
		!isHTTPVideoURL(base) || base.User != nil {
		return "", fmt.Errorf("frozen video base url is invalid")
	}
	if !strings.EqualFold(result.Scheme, base.Scheme) ||
		!strings.EqualFold(result.Hostname(), base.Hostname()) ||
		normalizedHTTPPort(result) != normalizedHTTPPort(base) {
		return "", fmt.Errorf("video result url is cross origin")
	}
	return resultURL, nil
}

func isHTTPVideoURL(parsed *url.URL) bool {
	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

func normalizedHTTPPort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	if strings.EqualFold(parsed.Scheme, "http") {
		return "80"
	}
	return "443"
}
