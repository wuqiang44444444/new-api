package model

import (
	"os"
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This file proves the Seedance plugin version lock and deletion protection on
// real MySQL and PostgreSQL servers, where lockForUpdate emits SELECT ... FOR
// UPDATE and the SQLite write-reservation emulation does not apply. Tests are
// gated by DSN env vars and refuse non-empty databases; see the P1P2 record
// §7 three-database item.

var seedanceCrossDBTargets = []struct {
	name      string
	env       string
	database  common.DatabaseType
	dialector func(string) gorm.Dialector
}{
	{name: "mysql", env: "TEST_SEEDANCE_PLUGIN_MYSQL_DSN", database: common.DatabaseTypeMySQL, dialector: mysql.Open},
	{name: "postgres", env: "TEST_SEEDANCE_PLUGIN_POSTGRES_DSN", database: common.DatabaseTypePostgreSQL, dialector: postgres.Open},
}

func TestSeedancePluginVersionLockAcrossServerDatabases(t *testing.T) {
	for _, target := range seedanceCrossDBTargets {
		t.Run(target.name, func(t *testing.T) {
			dsn := os.Getenv(target.env)
			if dsn == "" {
				t.Skipf("%s is not configured", target.env)
			}
			db, err := gorm.Open(target.dialector(dsn), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			for _, table := range []any{&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}} {
				if db.Migrator().HasTable(table) {
					t.Skipf("refusing to use non-empty %s test database", target.name)
				}
			}

			previousDB := DB
			previousMainType := common.MainDatabaseType()
			previousLogType := common.LogDatabaseType()
			DB = db
			common.SetDatabaseTypes(target.database, target.database)
			t.Cleanup(func() {
				DB = previousDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				_ = db.Migrator().DropTable(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{})
				sqlDB, sqlErr := db.DB()
				if sqlErr == nil {
					_ = sqlDB.Close()
				}
			})
			require.NoError(t, db.AutoMigrate(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}))

			t.Run("deletion blocks on a concurrent prepared attempt", func(t *testing.T) {
				seedCrossDBVersion(t, "1.0.0")
				frozen, marshalErr := common.Marshal(seedanceFrozenConnectionPin{PluginKey: "seedance-link", PluginVersion: "1.0.0"})
				require.NoError(t, marshalErr)

				// 受理事务：先取版本锁，再在未提交事务内写入 prepared attempt。
				tx := db.Begin()
				require.NoError(t, tx.Error)
				require.NoError(t, lockSeedancePluginVersion(tx, "seedance-link", "1.0.0"))
				attempt := &TaskCreateAttempt{
					AttemptID: "attempt_lock_race", PublicTaskID: "public_lock_race", UserID: 1,
					ClientProtocol: TaskClientProtocolModelArkV3, RequestHash: "hash",
					UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptPrepared,
					BillingHoldState:         TaskCreateAttemptBillingUnheld,
					FrozenConnectionSnapshot: frozen,
				}
				require.NoError(t, tx.Create(attempt).Error)

				// 删除必须等到受理提交后才能读到引用并拒绝。
				deleted := make(chan error, 1)
				go func() {
					_, deleteErr := DeleteTaskPluginVersion("seedance-link", "1.0.0")
					deleted <- deleteErr
				}()
				require.NoError(t, tx.Commit().Error)

				var deleteErr error
				require.NoError(t, waitForChannel(t, deleted, &deleteErr))
				var inUse *SeedancePluginInUseError
				require.ErrorAs(t, deleteErr, &inUse)
				assert.Empty(t, inUse.Tasks)
				require.Len(t, inUse.Attempts, 1)
				assert.Equal(t, int64(attempt.ID), inUse.Attempts[0].Id)
				assert.Equal(t, string(TaskCreateAttemptPrepared), inUse.Attempts[0].Status)

				var attemptCount, versionCount int64
				require.NoError(t, db.Model(&TaskCreateAttempt{}).Count(&attemptCount).Error)
				require.NoError(t, db.Model(&TaskPlugin{}).Count(&versionCount).Error)
				assert.EqualValues(t, 1, attemptCount)
				assert.EqualValues(t, 1, versionCount)
			})

			t.Run("committed deletion makes admission fail closed", func(t *testing.T) {
				seedCrossDBVersion(t, "1.1.0")
				_, deleteErr := DeleteTaskPluginVersion("seedance-link", "1.1.0")
				require.NoError(t, deleteErr)

				frozen, marshalErr := common.Marshal(seedanceFrozenConnectionPin{PluginKey: "seedance-link", PluginVersion: "1.1.0"})
				require.NoError(t, marshalErr)
				_, attemptErr := CreatePreparedTaskAttempt(TaskCreateAttemptParams{
					UserID: 1, PublicTaskID: "public_after_delete", ClientProtocol: TaskClientProtocolModelArkV3,
					RequestHash: "hash", UpstreamProtocol: "feicai_videos_v1", FrozenConnectionSnapshot: frozen,
					TaskPlugin: &TaskPluginSnapshot{Key: "seedance-link", Version: "1.1.0"},
				})
				require.Error(t, attemptErr)
				var attemptCount int64
				require.NoError(t, db.Model(&TaskCreateAttempt{}).
					Where("public_task_id = ?", "public_after_delete").Count(&attemptCount).Error)
				assert.Zero(t, attemptCount)
			})

			t.Run("dependency query matches frozen identity on server SQL", func(t *testing.T) {
				seedCrossDBVersion(t, "1.2.0")
				frozen := func(version string) []byte {
					raw, marshalErr := common.Marshal(seedanceFrozenConnectionPin{PluginKey: "seedance-link", PluginVersion: version})
					require.NoError(t, marshalErr)
					return raw
				}
				insertCrossDBTask := func(id int64, taskID, status, billing string, clientDeleted int64, version string) {
					task := &Task{TaskID: taskID, Platform: "62", UserId: 1, Status: TaskStatus(status)}
					task.PrivateData.Execution = &TaskExecutionSnapshot{
						TaskPlugin: &TaskPluginSnapshot{Key: "seedance-link", Version: version},
					}
					task.BillingState = TaskBillingState(billing)
					task.ClientDeletedAt = clientDeleted
					require.NoError(t, db.Create(task).Error)
					_ = id
				}
				insertCrossDBTask(1, "task_visible_success", string(TaskStatusSuccess), "settled", 0, "1.2.0")
				insertCrossDBTask(2, "task_deleted_success", string(TaskStatusSuccess), "settled", 1, "1.2.0")
				insertCrossDBTask(3, "task_settled_failure", string(TaskStatusFailure), "settled", 0, "1.2.0")
				insertCrossDBTask(4, "task_pending_failure", string(TaskStatusFailure), "pending", 0, "1.2.0")
				insertCrossDBTask(5, "task_settled_cancelled", string(TaskStatusCancelled), "settled", 0, "1.2.0")
				insertCrossDBTask(6, "task_running", string(TaskStatusInProgress), "", 0, "1.2.0")
				other := &Task{TaskID: "task_other_key", Platform: "62", UserId: 1, Status: TaskStatusInProgress}
				other.PrivateData.Execution = &TaskExecutionSnapshot{
					TaskPlugin: &TaskPluginSnapshot{Key: "doubao", Version: "9.0.0"},
				}
				require.NoError(t, db.Create(other).Error)

				attempt := &TaskCreateAttempt{
					AttemptID: "attempt_pinned", PublicTaskID: "public_pinned", UserID: 1,
					ClientProtocol: TaskClientProtocolModelArkV3, RequestHash: "hash",
					UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptUnknown,
					BillingHoldState:         TaskCreateAttemptBillingHeld,
					FrozenConnectionSnapshot: frozen("1.2.0"),
				}
				require.NoError(t, db.Create(attempt).Error)
				otherFrozen, marshalErr := common.Marshal(seedanceFrozenConnectionPin{PluginKey: "seedance-link", PluginVersion: "8.8.8"})
				require.NoError(t, marshalErr)
				unmatched := &TaskCreateAttempt{
					AttemptID: "attempt_other_version", PublicTaskID: "public_other_version", UserID: 1,
					ClientProtocol: TaskClientProtocolModelArkV3, RequestHash: "hash",
					UpstreamProtocol: "feicai_videos_v1", Status: TaskCreateAttemptUnknown,
					BillingHoldState:         TaskCreateAttemptBillingHeld,
					FrozenConnectionSnapshot: otherFrozen,
				}
				require.NoError(t, db.Create(unmatched).Error)

				tasks, attempts, err := GetSeedancePluginExecutionUsage("seedance-link", "1.2.0")
				require.NoError(t, err)
				taskIDs := make([]string, 0, len(tasks))
				for _, ref := range tasks {
					taskIDs = append(taskIDs, ref.TaskId)
				}
				sort.Strings(taskIDs)
				assert.Equal(t, []string{"task_pending_failure", "task_running", "task_visible_success"}, taskIDs)
				require.Len(t, attempts, 1)
				assert.Equal(t, attempt.ID, attempts[0].Id)
				assert.Equal(t, string(TaskCreateAttemptUnknown), attempts[0].Status)
			})
		})
	}
}

func seedCrossDBVersion(t *testing.T, version string) {
	t.Helper()
	row := TaskPlugin{Key: "seedance-link", Version: version, SourceHash: "fixture"}
	require.NoError(t, DB.Create(&row).Error)
}

func waitForChannel(t *testing.T, ch chan error, into *error) error {
	t.Helper()
	select {
	case err := <-ch:
		*into = err
		return nil
	case <-t.Context().Done():
		return t.Context().Err()
	}
}
