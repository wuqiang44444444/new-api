package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceReadabilityAcrossStoreRestart(t *testing.T) {
	oldConfig, oldStore := system_setting.GetTaskRequestEvidenceConfig(), evidenceObjectStore
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(oldConfig); evidenceObjectStore = oldStore })
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32), RetentionDays: 7, MaxBodyBytes: 1024, MaxResponseBytes: 1024, WriteTimeoutSeconds: 5}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, InitTaskRequestEvidenceStore(config))
	store := GetTaskRequestEvidenceStore()
	payload := []byte(`{"url":"https://example.test/video?sig=private"}`)
	key := "test/body"
	require.NoError(t, store.Put(key, payload))
	event := &model.TaskRequestEvidenceEvent{Id: 1, ObjectKey: key, Sha256: EvidenceSha256Hex(payload), ContentType: "application/json", Complete: true}
	sealed, err := os.ReadFile(filepath.Join(config.StorageDir, key))
	require.NoError(t, err)

	// Reinitializing with the same configured key must preserve existing objects.
	require.NoError(t, InitTaskRequestEvidenceStore(config))
	read, status := ReadEvidenceEventBody(event, false)
	assert.Equal(t, "available", status)
	assert.Equal(t, payload, read)

	wrongConfig := config
	wrongConfig.EncryptionKeyHex = strings.Repeat("02", 32)
	require.NoError(t, InitTaskRequestEvidenceStore(wrongConfig))
	read, status = ReadEvidenceEventBody(event, false)
	assert.Nil(t, read)
	assert.Equal(t, "decrypt_failed", status)
	assert.True(t, event.Complete, "current read failure must not rewrite capture facts")
	after, err := os.ReadFile(filepath.Join(config.StorageDir, key))
	require.NoError(t, err)
	assert.Equal(t, sealed, after)
	require.NoError(t, InitTaskRequestEvidenceStore(config))

	for _, tc := range []struct {
		name, contentType, digest, key string
		id                             int64
		expired                        bool
		want                           string
	}{
		{"binary", "video/mp4; codecs=h264", event.Sha256, key, 2, false, "available"},
		{"missing", "application/json", event.Sha256, "test/missing", 2, false, "missing"},
		{"digest mismatch", "application/json", "wrong", key, 2, false, "integrity_failed"},
		// An event row without an object is "missing"; "not_recorded" requires no row at all.
		{"row without object", "application/json", "", "", 2, false, "missing"},
		{"not captured", "application/json", "", "", 0, false, "not_recorded"},
		{"expired", "application/json", event.Sha256, key, 2, true, "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := &model.TaskRequestEvidenceEvent{Id: tc.id, ObjectKey: tc.key, ContentType: tc.contentType, Sha256: tc.digest}
			_, status := ReadEvidenceEventContent(item, tc.expired)
			assert.Equal(t, tc.want, status)
		})
	}
	// Empty but valid text must not be mistaken for unreadable evidence.
	require.NoError(t, store.Put("test/empty", nil))
	_, status = ReadEvidenceEventBody(&model.TaskRequestEvidenceEvent{ObjectKey: "test/empty", Sha256: EvidenceSha256Hex(nil)}, false)
	assert.Equal(t, "available", status)
	// Invalid ciphertext and filesystem errors remain distinct from missing data.
	require.NoError(t, os.WriteFile(filepath.Join(config.StorageDir, key), []byte("short"), 0600))
	_, status = ReadEvidenceEventBody(event, false)
	assert.Equal(t, "integrity_failed", status)
	_, status = ReadEvidenceEventBody(&model.TaskRequestEvidenceEvent{ObjectKey: "test"}, false)
	assert.Equal(t, "read_failed", status)
	// 关闭采集只停止新增正文：期限内历史读取继续，不被采集开关阻断。
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	_, status = ReadEvidenceEventBody(event, false)
	assert.Equal(t, "integrity_failed", status)
}
