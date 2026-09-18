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
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32), MaxBodyBytes: 1024, MaxResponseBytes: 1024, WriteTimeoutSeconds: 5}
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
	previews := GetEvidenceEventPreviews([]*model.TaskRequestEvidenceEvent{event}, false, false)
	assert.NotContains(t, previews[1].Text, "private")
	assert.Equal(t, "available", previews[1].BodyStatus)

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
		expired                        bool
		want                           string
	}{
		{"binary", "video/mp4; codecs=h264", event.Sha256, key, false, "binary"},
		{"missing", "application/json", event.Sha256, "test/missing", false, "missing"},
		{"digest mismatch", "application/json", "wrong", key, false, "integrity_failed"},
		{"not captured", "application/json", "", "", false, "not_recorded"},
		{"expired", "application/json", event.Sha256, key, true, "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := &model.TaskRequestEvidenceEvent{Id: 2, ObjectKey: tc.key, ContentType: tc.contentType, Sha256: tc.digest}
			preview := GetEvidenceEventPreviews([]*model.TaskRequestEvidenceEvent{item}, false, tc.expired)[2]
			assert.Equal(t, tc.want, preview.BodyStatus)
			assert.Empty(t, preview.Text)
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
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	_, status = ReadEvidenceEventBody(event, false)
	assert.Equal(t, "storage_unavailable", status)
}
