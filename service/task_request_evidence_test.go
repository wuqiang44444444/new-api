package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceStoreRoundtripEncrypted(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	config := system_setting.TaskRequestEvidenceConfig{
		Enabled:          true,
		RetentionDays:    7,
		StorageDir:       dir,
		EncryptionKeyHex: hex.EncodeToString(key),
		MaxBodyBytes:     1 << 20,
		MaxResponseBytes: 1 << 20,
	}
	store, err := newLocalEncryptedEvidenceStoreForTest(config)
	require.NoError(t, err)

	payload := []byte(`{"prompt":"hello","generate_audio":true}`)
	require.NoError(t, store.Put("1/1.bin", payload))

	// 磁盘上的字节必须是密文，不得出现明文业务内容。
	raw, err := os.ReadFile(filepath.Join(dir, "1", "1.bin"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "hello")

	roundtrip, err := store.Get("1/1.bin")
	require.NoError(t, err)
	assert.Equal(t, payload, roundtrip)

	_, deleteErr := store.Delete("1/1.bin")
	require.NoError(t, deleteErr)
	_, err = store.Get("1/1.bin")
	assert.Error(t, err)
}

func TestEvidenceResponseBodyTeeCapturesWhileForwarding(t *testing.T) {
	response := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
	}
	tee := newEvidenceResponseBodyTeeForTest(t, response, 1<<20)
	response.Body = tee
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"status":"ok"}`, string(body))
	require.NoError(t, response.Body.Close())
}

func TestEvidenceMaskSignedURLs(t *testing.T) {
	masked := EvidenceMaskSignedURLs("https://cdn.example/v.mp4?X-Amz-Signature=abcdef&expires=123")
	assert.Contains(t, masked, "https://cdn.example/v.mp4")
	assert.NotContains(t, masked, "abcdef")
	assert.NotContains(t, masked, "123")
	assert.Equal(t, "plain text", EvidenceMaskSignedURLs("plain text"))
}

// --- 测试夹具辅助 ---

func newLocalEncryptedEvidenceStoreForTest(config system_setting.TaskRequestEvidenceConfig) (TaskRequestEvidenceObjectStore, error) {
	rawKey, err := hex.DecodeString(config.EncryptionKeyHex)
	if err != nil || len(rawKey) != 32 {
		return nil, aes.KeySizeError(len(rawKey))
	}
	block, err := aes.NewCipher(rawKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &localEncryptedEvidenceStore{baseDir: config.StorageDir, aead: gcm}, nil
}

func newEvidenceResponseBodyTeeForTest(t *testing.T, response *http.Response, maxBytes int64) *evidenceResponseBodyTee {
	t.Helper()
	return &evidenceResponseBodyTee{
		session:  &taskRequestEvidenceSession{},
		origin:   response.Body,
		buffer:   bytes.NewBuffer(nil),
		maxBytes: maxBytes,
		started:  0,
	}
}
