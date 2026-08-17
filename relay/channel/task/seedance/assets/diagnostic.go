package assets

import (
	"errors"
	"fmt"
)

// SafeUpstreamDiagnostic returns only bounded, non-sensitive fields from known
// asset adapter errors. Callers must not log the original error or response body.
func SafeUpstreamDiagnostic(err error) (string, bool) {
	var statusErr *upstreamHTTPError
	if errors.As(err, &statusErr) {
		providerCode := statusErr.ProviderCode
		if len(providerCode) > 128 {
			providerCode = ""
		}
		for _, character := range providerCode {
			if character >= 'a' && character <= 'z' ||
				character >= 'A' && character <= 'Z' ||
				character >= '0' && character <= '9' ||
				character == '.' || character == '_' || character == '-' {
				continue
			}
			providerCode = ""
			break
		}
		if providerCode != "" {
			return fmt.Sprintf("status=%d provider_code=%s", statusErr.StatusCode, providerCode), true
		}
		return fmt.Sprintf("status=%d", statusErr.StatusCode), true
	}

	var applicationErr *upstreamApplicationError
	if errors.As(err, &applicationErr) {
		return fmt.Sprintf("provider_code=%d", applicationErr.code), true
	}
	return "", false
}
