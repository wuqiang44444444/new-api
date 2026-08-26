package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetChannelUpstreamErrorDoesNotExposeSignedRequestURL(t *testing.T) {
	cause := errors.New(`Post "https://provider.example/asset/query?AccessKey=secret&Signature=secret": EOF`)

	err := assetChannelUpstreamError(cause)

	require.Error(t, err)
	assert.Equal(t, "asset upstream is unavailable", err.Error())
	assert.Equal(t, ChannelConnectivityAssetUnavailable, ChannelConnectivityErrorCode(err))
	assert.ErrorIs(t, err, cause)
	assert.NotContains(t, err.Error(), "provider.example")
	assert.NotContains(t, err.Error(), "AccessKey")
	assert.NotContains(t, err.Error(), "Signature")
}

func TestAssetChannelConfigurationErrorUsesStablePublicMessages(t *testing.T) {
	tests := []struct {
		name    string
		cause   error
		message string
		code    string
	}{
		{
			name:    "not configured",
			cause:   ErrAssetLibraryUnsupported,
			message: "asset action is not configured",
			code:    ChannelConnectivityAssetNotConfigured,
		},
		{
			name:    "invalid configuration",
			cause:   ErrAssetUpstreamUnavailable,
			message: "asset action configuration is invalid",
			code:    ChannelConnectivityAssetInvalidConfig,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := assetChannelConfigurationError(test.cause)
			require.Error(t, err)
			assert.Equal(t, test.message, err.Error())
			assert.Equal(t, test.code, ChannelConnectivityErrorCode(err))
			assert.ErrorIs(t, err, test.cause)
		})
	}
}
