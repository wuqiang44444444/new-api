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

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceRedactHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": {"Bearer sk-test"},
		"Cookie":        {"session=abc"},
		"Content-Type":  {"application/json"},
		"X-Api-Key":     {"secret-key"},
	}
	redacted := EvidenceRedactHeaders(headers)
	assert.Equal(t, "[REDACTED]", redacted["Authorization"])
	assert.Equal(t, "[REDACTED]", redacted["Cookie"])
	assert.Equal(t, "[REDACTED]", redacted["X-Api-Key"])
	assert.Equal(t, "application/json", redacted["Content-Type"])
}

func TestEvidenceRedactJSONBodyPreservesBusinessFields(t *testing.T) {
	body := []byte(`{"model":"seedance-1","max_tokens":4096,"prompt_tokens_hint":2,` +
		`"api_key":"sk-secret","nested":{"access_token":"tok","keep":"yes"},"prompt_tokens":100}`)
	redacted, err := evidenceRedactBody(body, "application/json")
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, common.Unmarshal(redacted, &out))
	// 业务字段完整保留：max_tokens 等含 token 字样的合法字段不得被误删。
	assert.Equal(t, float64(4096), out["max_tokens"])
	assert.Equal(t, float64(100), out["prompt_tokens"])
	assert.Equal(t, "seedance-1", out["model"])
	// 凭据字段保持存在性，但值被脱敏。
	assert.Equal(t, "[REDACTED]", out["api_key"])
	nested, ok := out["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "[REDACTED]", nested["access_token"])
	assert.Equal(t, "yes", nested["keep"])
	// prompt_tokens_hint 不以 _token 结尾（复数），必须保留。
	assert.Equal(t, float64(2), out["prompt_tokens_hint"])
}

func TestEvidenceRedactQueryParams(t *testing.T) {
	values := map[string][]string{
		"access_token": {"secret"},
		"model":        {"seedance-1"},
	}
	redacted := EvidenceRedactQueryParams(values)
	assert.Equal(t, "[REDACTED]", redacted["access_token"])
	assert.Equal(t, "seedance-1", redacted["model"])
}

func TestEvidenceStoreRoundtripEncrypted(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	config := system_setting.TaskRequestEvidenceConfig{
		Enabled:          true,
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

	require.NoError(t, store.Delete("1/1.bin"))
	_, err = store.Get("1/1.bin")
	assert.Error(t, err)
}

func TestEvidenceSnapshotRequestBodyPreservesReplay(t *testing.T) {
	original := []byte(`{"with_audio":true}`)
	request, err := http.NewRequest(http.MethodPost, "https://provider.example/v3/media", bytes.NewReader(original))
	require.NoError(t, err)
	require.NotNil(t, request.GetBody)

	snapshot, err := evidenceSnapshotRequestBody(request, 1<<20)
	require.NoError(t, err)
	assert.Equal(t, original, snapshot)

	// GetBody 路径不消费原始 Body：重放契约保持不变。
	replayed, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	assert.Equal(t, original, replayed)
}

func TestEvidenceSnapshotRequestBodyRejectsOversize(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://provider.example", bytes.NewReader(make([]byte, 4096)))
	require.NoError(t, err)
	_, err = evidenceSnapshotRequestBody(request, 1024)
	require.Error(t, err)
	require.True(t, IsTaskRequestEvidenceUnavailable(err))
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
	assert.True(t, tee.eof)
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
		observed: 0,
	}
}
