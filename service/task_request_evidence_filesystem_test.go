package service

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceFileWalkDeletesAcrossPagesWithoutFollowingDirectories(t *testing.T) {
	setupEvidenceTestEnv(t)
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	dir := filepath.Join(store.baseDir, "7")
	require.NoError(t, os.MkdirAll(dir, 0700))
	for id := 1; id <= 513; id++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, strconv.Itoa(id)+".bin"), []byte("fixture"), 0600))
	}
	protected := filepath.Join(dir, "nested", "untouched.bin")
	require.NoError(t, os.MkdirAll(filepath.Dir(protected), 0700))
	require.NoError(t, os.WriteFile(protected, []byte("outside non-recursive inventory"), 0600))
	var deleted int
	require.NoError(t, store.WalkEvidenceFiles(context.Background(), 7, func(file EvidenceStoredFile) error {
		_, err := store.Delete(file.Key)
		deleted++
		return err
	}))
	assert.Equal(t, 513, deleted)
	assert.FileExists(t, protected)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	err = store.RemoveEmptyEvidenceDir(7)
	var nonEmpty *EvidenceDirectoryNotEmptyError
	require.ErrorAs(t, err, &nonEmpty)
	assert.Equal(t, 1, nonEmpty.Files)
}
