package controller

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeVideoProviderErrorRemovesRequestURLAndSecrets(t *testing.T) {
	const key = "AIza-provider-secret"
	err := &url.Error{
		Op:  "Get",
		URL: "https://provider.example/video?key=" + key,
		Err: errors.New("dial failed for " + key),
	}

	message := sanitizeVideoProviderError(err, key)

	assert.Equal(t, "dial failed for [REDACTED]", message)
	assert.NotContains(t, message, "provider.example")
	assert.NotContains(t, message, key)
}
