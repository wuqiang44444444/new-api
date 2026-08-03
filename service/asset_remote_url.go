package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type AssetURLTTLInsufficientError struct {
	RequiredMinTTLSeconds int64
}

func (e *AssetURLTTLInsufficientError) Error() string {
	return fmt.Sprintf("asset URL requires at least %d seconds of remaining TTL", e.RequiredMinTTLSeconds)
}

func (e *AssetURLTTLInsufficientError) Unwrap() error { return ErrAssetURLTTLInsufficient }

func RequiredAssetURLTTL(err error) (int64, bool) {
	var ttlErr *AssetURLTTLInsufficientError
	if !errors.As(err, &ttlErr) {
		return 0, false
	}
	return ttlErr.RequiredMinTTLSeconds, true
}

func validateRemoteAssetURL(rawURL string, maxLength int) (string, error) {
	remoteURL, err := normalizeRemoteAssetURL(rawURL)
	if err != nil {
		return "", err
	}
	if len(remoteURL) > maxLength {
		return "", fmt.Errorf("%w: URL is empty or too long", ErrUnsafeAssetURL)
	}
	protection, err := common.NewSSRFProtectionFromFetchSetting(false, false, false, nil, nil, []string{"443"}, true)
	if err != nil {
		return "", err
	}
	if err := protection.ValidateURL(remoteURL); err != nil {
		return "", fmt.Errorf("%w: target is not a public network address", ErrUnsafeAssetURL)
	}
	return remoteURL, nil
}

// normalizeRemoteAssetURL performs only stable syntax normalization so an
// idempotent replay is not blocked by mutable routing or SSRF configuration.
func normalizeRemoteAssetURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("%w: URL must be an absolute HTTPS URL without userinfo or fragment", ErrUnsafeAssetURL)
	}
	if parsed.Port() != "" && parsed.Port() != "443" {
		return "", fmt.Errorf("%w: HTTPS URL must use port 443", ErrUnsafeAssetURL)
	}
	return parsed.String(), nil
}

func validateRemoteAssetTTL(expiresAt, minimumTTLSeconds int64, now time.Time) error {
	if expiresAt == 0 {
		return nil
	}
	if expiresAt <= now.Unix() || expiresAt-now.Unix() < minimumTTLSeconds {
		return &AssetURLTTLInsufficientError{RequiredMinTTLSeconds: minimumTTLSeconds}
	}
	return nil
}
