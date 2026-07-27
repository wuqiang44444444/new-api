package controller

import (
	"errors"
	"net/url"
	"strings"
)

func sanitizeVideoProviderError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}
	message := err.Error()
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(secret), "[REDACTED]")
		message = strings.ReplaceAll(message, url.PathEscape(secret), "[REDACTED]")
	}
	return message
}
