package common

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateHTTPSVideoResultURL validates cross-origin provider result URLs.
// The content proxy performs the subsequent DNS/IP checks before fetching.
func ValidateHTTPSVideoResultURL(rawResultURL string) (string, error) {
	resultURL := strings.TrimSpace(rawResultURL)
	if resultURL == "" || len(resultURL) > maxSameOriginVideoResultURLLength {
		return "", fmt.Errorf("video result url is missing or too long")
	}
	parsed, err := url.Parse(resultURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.Opaque != "" ||
		!strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil {
		return "", fmt.Errorf("video result url must be an absolute https url without userinfo")
	}
	return resultURL, nil
}
