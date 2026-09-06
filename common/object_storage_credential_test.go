package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageCredentialEnvelope(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "master-key")
	setCryptoSecret := func(value string) {
		previous := CryptoSecret
		CryptoSecret = value
		t.Cleanup(func() { CryptoSecret = previous })
	}

	t.Run("empty credential passes through", func(t *testing.T) {
		setCryptoSecret("master-key")
		ciphertext, err := EncryptObjectStorageCredential("")
		require.NoError(t, err)
		assert.Empty(t, ciphertext)
	})

	t.Run("roundtrip with domain separated prefix", func(t *testing.T) {
		setCryptoSecret("master-key")
		ciphertext, err := EncryptObjectStorageCredential("az-account-key===")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(ciphertext, "objstore.v1."))
		plaintext, err := DecryptObjectStorageCredential(ciphertext)
		require.NoError(t, err)
		assert.Equal(t, "az-account-key===", plaintext)
	})

	t.Run("mismatched master key fails closed", func(t *testing.T) {
		setCryptoSecret("master-key")
		ciphertext, err := EncryptObjectStorageCredential("az-account-key")
		require.NoError(t, err)
		setCryptoSecret("another-node-key")
		_, err = DecryptObjectStorageCredential(ciphertext)
		require.Error(t, err)
	})

	t.Run("foreign ciphertext is rejected", func(t *testing.T) {
		setCryptoSecret("master-key")
		_, err := DecryptObjectStorageCredential("v2.abc")
		require.Error(t, err)
		_, err = DecryptObjectStorageCredential("not-a-ciphertext")
		require.Error(t, err)
	})
}

func TestObjectStorageCredentialRequiresStableMasterKey(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	ciphertext, err := EncryptObjectStorageCredential("secret")
	require.Error(t, err)
	assert.Empty(t, ciphertext)
	t.Setenv("SESSION_SECRET", "stable-session-secret")
	ciphertext, err = EncryptObjectStorageCredential("secret")
	require.NoError(t, err)
	value, err := DecryptObjectStorageCredential(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, "secret", value)
}
