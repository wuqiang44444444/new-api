package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// These DSNs must name disposable, empty databases. They are intentionally
// separate from application SQL_DSN so tests never select production storage.
func TestEvidenceCleanupExternalDatabaseContracts(t *testing.T) {
	for _, database := range []common.DatabaseType{common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(database), func(t *testing.T) {
			key := "EVIDENCE_TEST_MYSQL_DSN"
			if database == common.DatabaseTypePostgreSQL {
				key = "EVIDENCE_TEST_POSTGRES_DSN"
			}
			dsn := os.Getenv(key)
			if dsn == "" {
				t.Skip("disposable evidence database DSN not configured")
			}
			var dialector gorm.Dialector = mysql.Open(dsn)
			if database == common.DatabaseTypePostgreSQL {
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(database)
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				require.NoError(t, sqlDB.Close())
			})
			require.False(t, db.Migrator().HasTable(&TaskRequestEvidence{}), "test database must be empty")
			require.NoError(t, db.AutoMigrate(&evidenceBeforeCleanup{}))
			now := common.GetTimestamp()
			old := &evidenceBeforeCleanup{CreatedAt: now - 3600}
			require.NoError(t, db.Create(old).Error)
			require.NoError(t, db.AutoMigrate(&TaskRequestEvidence{}, &TaskRequestEvidenceEvent{}, &TaskRequestEvidenceAccessLog{}, &Task{}, &SystemTask{}, &SystemTaskLock{}))
			require.NoError(t, db.Model(&TaskRequestEvidence{}).Where("id = ?", old.Id).UpdateColumn("cleanup_requested_at", nil).Error)
			require.NoError(t, RequestTaskRequestEvidenceCleanup(old.Id, now))
			index, exists, err := GetTaskRequestEvidenceById(old.Id)
			require.NoError(t, err)
			require.True(t, exists)
			assert.Equal(t, now, index.CleanupRequestedAt)
			items, err := TaskRequestEvidenceExpiredBodies(now, 7*86400, 0, 0, 100)
			require.NoError(t, err)
			require.Len(t, items, 1)
			assert.Equal(t, old.Id, items[0].Id)
			assert.ErrorIs(t, CommitTaskRequestEvidenceObject(old.Id, 7*86400, func() error { t.Fatal("closed index must not publish an object"); return nil }), ErrTaskRequestEvidenceBodyClosed)
			before := &TaskRequestEvidence{CreatedAt: now - 1800}
			at := &TaskRequestEvidence{CreatedAt: now - 900}
			require.NoError(t, CreateTaskRequestEvidence(before))
			require.NoError(t, CreateTaskRequestEvidence(at))
			cutoff := now - 900
			task, err := CreateEvidenceCleanupSystemTask(cutoff, map[string]any{"target_timestamp": cutoff, "operator_user_id": 77, "trigger": "manual"})
			require.NoError(t, err)
			index, _, err = GetTaskRequestEvidenceById(before.Id)
			require.NoError(t, err)
			assert.NotZero(t, index.CleanupRequestedAt)
			index, _, err = GetTaskRequestEvidenceById(at.Id)
			require.NoError(t, err)
			assert.Zero(t, index.CleanupRequestedAt)
			_, claimed, err := ClaimSystemTask(task.ID, task.Type, "contract-runner", now+3600)
			require.NoError(t, err)
			require.True(t, claimed)
			require.NoError(t, FinishSystemTask(task.TaskID, "contract-runner", SystemTaskStatusFailed, nil, "cancelled"))
			items, err = TaskRequestEvidenceExpiredBodies(now, 7*86400, 0, 0, 100)
			require.NoError(t, err)
			require.Len(t, items, 2)
			health, err := GetEvidenceCleanupHealth(now, 7*86400)
			require.NoError(t, err)
			assert.EqualValues(t, 2, health.PendingIndexes)
			assert.Equal(t, 1, health.ConsecutiveFailures)
			runEvidenceDatabaseDeletionContract(t)
			runMidjourneyEvidenceAnchorContract(t)
		})
	}
}
