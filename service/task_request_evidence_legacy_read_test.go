package service

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceLegacyPreviewIsBoundedAndChecksCompleteObject(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	body := bytes.Repeat([]byte("opaque original bytes\n"), 10000)
	payload := append([]byte(evidenceObjectMagic+"\n{\"content_type\":\"application/json\"}\n"), body...)
	nonce := bytes.Repeat([]byte{3}, store.aead.NonceSize())
	sealed := store.aead.Seal(append([]byte(nil), nonce...), nonce, payload, nil)
	path := filepath.Join(store.baseDir, "legacy.bin")
	require.NoError(t, os.WriteFile(path, sealed, 0600))
	event := &model.TaskRequestEvidenceEvent{Id: 1, ObjectKey: "legacy.bin", Sha256: EvidenceSha256Hex(payload)}
	content, status := ReadEvidenceEventContent(event, false)
	require.Equal(t, "available", status)
	require.NotNil(t, content.Meta)
	assert.Equal(t, "application/json", content.Meta.ContentType)
	assert.Equal(t, body[:64<<10], content.Body)
	assert.True(t, content.DisplayLimited)
	// Authentication and the historical digest must also cover bytes beyond
	// the visible preview. Replace the file with a valid different ciphertext.
	payload[len(payload)-1] ^= 1
	require.NoError(t, os.WriteFile(path, store.aead.Seal(append([]byte(nil), nonce...), nonce, payload, nil), 0600))
	_, status = ReadEvidenceEventContent(event, false)
	assert.Equal(t, "integrity_failed", status)
	require.NoError(t, os.WriteFile(path, sealed, 0600))
	reader, status := OpenEvidenceEventBody(event, false)
	require.Equal(t, "available", status)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	_, status = ReadEvidenceEventContent(event, false)
	assert.Equal(t, "storage_unavailable", status, "a held legacy reader owns its memory lease")
	require.NoError(t, reader.Close())
	_, status = ReadEvidenceEventContent(event, false)
	assert.Equal(t, "available", status, "closing releases the lease exactly once")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, sealed, after, "reading never rewrites historical ciphertext")
}

func TestEvidenceLegacyOversizeReadRejectsBeforeDecryptionAndKeepsStreaming(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	path := filepath.Join(store.baseDir, "oversize.bin")
	file, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, file.Truncate(evidenceLegacyMaxBytes+1))
	require.NoError(t, file.Close())
	_, status := OpenEvidenceEventBody(&model.TaskRequestEvidenceEvent{ObjectKey: "oversize.bin"}, false)
	assert.Equal(t, "storage_unavailable", status)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.EqualValues(t, evidenceLegacyMaxBytes+1, info.Size())
	require.NoError(t, store.Put("stream.bin", []byte("stream remains readable")))
	reader, status := OpenEvidenceEventBody(&model.TaskRequestEvidenceEvent{ObjectKey: "stream.bin"}, false)
	require.Equal(t, "available", status)
	defer reader.Close()
	actual, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "stream remains readable", string(actual))
}

func TestEvidenceLegacyPreviewUsesSharedReadLimit(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	require.NoError(t, store.Put("stream.bin", []byte("fixture")))
	var readers []io.ReadCloser
	t.Cleanup(func() {
		for _, reader := range readers {
			require.NoError(t, reader.Close())
		}
	})
	for i := 0; i < 8; i++ {
		reader, status := OpenEvidenceEventBody(&model.TaskRequestEvidenceEvent{ObjectKey: "stream.bin"}, false)
		require.Equal(t, "available", status)
		readers = append(readers, reader)
	}
	_, status := ReadEvidenceEventContent(&model.TaskRequestEvidenceEvent{ObjectKey: "not-opened.bin", Sha256: "historical"}, false)
	assert.Equal(t, "storage_unavailable", status, "historical digest cannot bypass the shared reader limit")
}
