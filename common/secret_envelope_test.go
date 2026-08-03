package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopedShortLivedSecretBindsCiphertextToScopeAndReadsLegacy(t *testing.T) {
	previous := CryptoSecret
	CryptoSecret = "scope-test-secret"
	t.Cleanup(func() { CryptoSecret = previous })

	ciphertext, err := EncryptShortLivedSecretForScope("authorization:1/session:2", "sensitive")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ciphertext, scopedSecretEnvelopePrefix))

	plaintext, err := DecryptShortLivedSecretForScope("authorization:1/session:2", ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "sensitive", plaintext)

	_, err = DecryptShortLivedSecretForScope("authorization:1/session:3", ciphertext)
	require.Error(t, err)

	legacy, err := EncryptShortLivedSecret("legacy-sensitive")
	require.NoError(t, err)
	plaintext, err = DecryptShortLivedSecretForScope("authorization:1/session:2", legacy)
	require.NoError(t, err)
	assert.Equal(t, "legacy-sensitive", plaintext)
}

func TestShortLivedSecretEnvelopeRoundTripAndTamperDetection(t *testing.T) {
	originalSecret := CryptoSecret
	CryptoSecret = "test-secret-envelope-key"
	t.Cleanup(func() { CryptoSecret = originalSecret })

	first, err := EncryptShortLivedSecret("short-lived-provider-token")
	require.NoError(t, err)
	second, err := EncryptShortLivedSecret("short-lived-provider-token")
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.NotContains(t, first, "short-lived-provider-token")

	plaintext, err := DecryptShortLivedSecret(first)
	require.NoError(t, err)
	assert.Equal(t, "short-lived-provider-token", plaintext)

	replacement := byte('A')
	if first[0] == replacement {
		replacement = 'B'
	}
	tampered := string(replacement) + first[1:]
	_, err = DecryptShortLivedSecret(tampered)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "short-lived-provider-token")
}
