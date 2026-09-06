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
	"os"
	"strings"
)

// 对象存储长期凭据的加密信封。主密钥从部署侧稳定主密钥（CRYPTO_SECRET /
// SESSION_SECRET）域分隔派生，不与数据库密文同表存储；本地与 HK、US 节点
// 必须配置一致的主密钥。它与短期凭据信封（secret_envelope.go）用途与域不同，
// 不复用“短期”语义冒充长期密钥管理；objstore.v1. 前缀为未来密钥轮换保留
// 版本化解码空间。

const objectStorageCredentialEnvelopePrefix = "objstore.v1."

func objectStorageCredentialKey() []byte {
	mac := hmac.New(sha256.New, []byte(CryptoSecret))
	_, _ = mac.Write([]byte("new-api/object-storage-credential/v1"))
	return mac.Sum(nil)
}

// EncryptObjectStorageCredential 加密对象存储长期凭据（Azure Account Key 或
// S3 Secret Key）。空值原样返回空。
func EncryptObjectStorageCredential(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if strings.TrimSpace(os.Getenv("CRYPTO_SECRET")) == "" && strings.TrimSpace(os.Getenv("SESSION_SECRET")) == "" {
		return "", fmt.Errorf("configure a stable CRYPTO_SECRET or SESSION_SECRET before saving storage credentials")
	}
	gcm, err := createObjectStorageGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return objectStorageCredentialEnvelopePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptObjectStorageCredential 解密对象存储凭据；主密钥不一致或密文
// 损坏时返回错误，调用方必须失败关闭（存储保持禁用并记录告警）。
func DecryptObjectStorageCredential(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	if !strings.HasPrefix(ciphertext, objectStorageCredentialEnvelopePrefix) {
		return "", fmt.Errorf("invalid encrypted storage credential")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, objectStorageCredentialEnvelopePrefix))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted storage credential")
	}
	gcm, err := createObjectStorageGCM()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid encrypted storage credential")
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted storage credential")
	}
	return string(plaintext), nil
}

func createObjectStorageGCM() (cipher.AEAD, error) {
	block, err := aes.NewCipher(objectStorageCredentialKey())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
