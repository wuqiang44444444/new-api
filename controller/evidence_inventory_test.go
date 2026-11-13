package controller

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvidenceInventoryPagesOrphansAndKeepsLiveNamespaces(t *testing.T) {
	store, dir := setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	live := []model.TaskRequestEvidence{{Id: 1, CreatedAt: now}, {Id: 300, CreatedAt: now}, {Id: 601, CreatedAt: now}}
	require.NoError(t, model.DB.Create(&live).Error)
	old := time.Unix(now-3*86400, 0)
	// Cross multiple inventory batches with live windows in different pages.
	for id := 1; id <= 601; id++ {
		path := filepath.Join(dir, strconv.Itoa(id), "orphan.bin")
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte("private fixture"), 0600))
		require.NoError(t, os.Chtimes(path, old, old))
	}
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "untouched.bin")
	require.NoError(t, os.WriteFile(outsideFile, []byte("outside evidence root"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "9000")))
	queries := 0
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("inventory-query-count", func(*gorm.DB) { queries++ }))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove("inventory-query-count")) })
	stats := &evidenceRetentionStats{Cutoff: now}
	require.NoError(t, cleanupEvidenceDatabase(context.Background(), store, stats, 2*86400, func() error { return nil }))
	assert.Less(t, queries, 20, "hundreds of namespaces must not trigger one lookup per namespace")
	assert.EqualValues(t, 598, stats.DeletedFiles)
	assert.Zero(t, stats.FailedItems)
	for _, index := range live {
		assert.FileExists(t, filepath.Join(dir, strconv.FormatInt(index.Id, 10), "orphan.bin"))
	}
	assert.FileExists(t, outsideFile)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 4, "only three live directories and the unfollowed symlink remain")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.WalkEvidenceNamespaces(ctx, func([]int64) error { t.Fatal("cancelled scan must not visit"); return nil })
	assert.ErrorIs(t, err, context.Canceled)
}
