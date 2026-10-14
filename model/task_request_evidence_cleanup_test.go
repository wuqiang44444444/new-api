package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type evidenceBeforeCleanup struct {
	Id          int64 `gorm:"primaryKey"`
	CreatedAt   int64
	RetainUntil int64
	BodyExpired bool
}

func (evidenceBeforeCleanup) TableName() string { return "task_request_evidences" }

func TestEvidenceCleanupMigrationPreservesOldRowsWithUsableMarker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.db")), &gorm.Config{})
	require.NoError(t, err)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.AutoMigrate(&evidenceBeforeCleanup{}))
	old := &evidenceBeforeCleanup{CreatedAt: common.GetTimestamp()}
	require.NoError(t, db.Create(old).Error)
	require.NoError(t, db.AutoMigrate(&TaskRequestEvidence{}))
	index, exists, err := GetTaskRequestEvidenceById(old.Id)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Zero(t, index.CleanupRequestedAt)
	require.NoError(t, RequestTaskRequestEvidenceCleanup(old.Id, common.GetTimestamp()))
	index, _, err = GetTaskRequestEvidenceById(old.Id)
	require.NoError(t, err)
	assert.NotZero(t, index.CleanupRequestedAt)
}

func TestEvidenceObjectCommitNeverPublishesClosedOrMissingIndex(t *testing.T) {
	for _, state := range []string{"requested", "completed", "expired", "missing", "open"} {
		t.Run(state, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commit.db")), &gorm.Config{})
			require.NoError(t, err)
			previous := DB
			DB = db
			t.Cleanup(func() { DB = previous })
			require.NoError(t, db.AutoMigrate(&TaskRequestEvidence{}))
			index := &TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
			if state == "requested" {
				index.CleanupRequestedAt = common.GetTimestamp()
			}
			if state == "completed" {
				index.BodyExpired = true
			}
			if state == "expired" {
				index.CreatedAt -= 8 * 86400
			}
			if state != "missing" {
				require.NoError(t, CreateTaskRequestEvidence(index))
			}
			path := filepath.Join(t.TempDir(), "published.bin")
			err = CommitTaskRequestEvidenceObject(index.Id, 7*86400, func() error { return os.WriteFile(path, []byte("body"), 0o600) })
			if state == "open" {
				require.NoError(t, err)
				body, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, "body", string(body))
			} else {
				require.Error(t, err)
				_, err = os.Stat(path)
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}
