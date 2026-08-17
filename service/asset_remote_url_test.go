package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRemoteAssetURLRequiresPublicHTTPSOnPort443(t *testing.T) {
	for _, rawURL := range []string{
		"http://example.com/source.png",
		"https://example.com:8443/source.png",
		"https://user@example.com/source.png",
		"https://example.com/source.png#fragment",
		"https://127.0.0.1/source.png",
	} {
		_, err := validateRemoteAssetURL(rawURL, 8192)
		require.ErrorIs(t, err, ErrUnsafeAssetURL, rawURL)
	}

	remoteURL, err := validateRemoteAssetURL(" https://1.1.1.1/source.png ", 8192)
	require.NoError(t, err)
	assert.Equal(t, "https://1.1.1.1/source.png", remoteURL)
}

func TestValidateRemoteAssetURLHonorsConfiguredLength(t *testing.T) {
	_, err := validateRemoteAssetURL("https://example.com/"+strings.Repeat("a", 32), 24)
	require.ErrorIs(t, err, ErrUnsafeAssetURL)
}

func TestValidateRemoteAssetTTLAllowsOmissionAndEnforcesRemainingTTL(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, validateRemoteAssetTTL(0, 3600, now))
	require.NoError(t, validateRemoteAssetTTL(now.Unix()+3600, 3600, now))

	err := validateRemoteAssetTTL(now.Unix()+3599, 3600, now)
	require.ErrorIs(t, err, ErrAssetURLTTLInsufficient)
	requiredTTL, ok := RequiredAssetURLTTL(err)
	require.True(t, ok)
	assert.Equal(t, int64(3600), requiredTTL)
}
