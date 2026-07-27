package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const scopedSecretEnvelopePrefix = "v2."

func EncryptShortLivedSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func DecryptShortLivedSecret(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	key := sha256.Sum256([]byte(CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	return string(plaintext), nil
}

// EncryptShortLivedSecretForScope uses a domain-separated key and binds the
// ciphertext to a stable record scope so it cannot be replayed in another
// verification session.
func EncryptShortLivedSecretForScope(scope, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(shortLivedSecretV2Key())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), []byte(scope))
	return scopedSecretEnvelopePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptShortLivedSecretForScope accepts legacy unscoped ciphertexts during
// rollout; all newly encrypted values use the scoped v2 envelope.
func DecryptShortLivedSecretForScope(scope, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, scopedSecretEnvelopePrefix) {
		return DecryptShortLivedSecret(ciphertext)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, scopedSecretEnvelopePrefix))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	block, err := aes.NewCipher(shortLivedSecretV2Key())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(scope))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted secret")
	}
	return string(plaintext), nil
}

func shortLivedSecretV2Key() []byte {
	mac := hmac.New(sha256.New, []byte(CryptoSecret))
	_, _ = mac.Write([]byte("new-api/short-lived-secret/v2"))
	return mac.Sum(nil)
}
