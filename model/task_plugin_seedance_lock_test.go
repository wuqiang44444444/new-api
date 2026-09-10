package model

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedancePluginAdmissionAndDeletionSerialize(t *testing.T) {
	for _, admissionFirst := range []bool{true, false} {
		name := "delete wins"
		if admissionFirst {
			name = "admission wins"
		}
		t.Run(name, func(t *testing.T) {
			originalDB := DB
			mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
			common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
			t.Cleanup(func() { DB = originalDB; common.SetDatabaseTypes(mainType, logType) })
			var err error
			DB, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "versions.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(4)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.NoError(t, DB.AutoMigrate(&TaskPlugin{}, &Task{}, &TaskCreateAttempt{}))
			row := TaskPlugin{Key: "seedance-link", Version: "1.0.0", SourceHash: "fixture"}
			require.NoError(t, DB.Create(&row).Error)
			frozen, err := common.Marshal(seedanceFrozenConnectionPin{PluginKey: row.Key, PluginVersion: row.Version})
			require.NoError(t, err)
			params := TaskCreateAttemptParams{UserID: 1, PublicTaskID: "public-task", ClientProtocol: TaskClientProtocolModelArkV3,
				RequestHash: "request", UpstreamProtocol: "feicai_videos_v1", FrozenConnectionSnapshot: frozen,
				TaskPlugin: &TaskPluginSnapshot{Key: row.Key, Version: row.Version}}
			admit := func() error { _, err := CreatePreparedTaskAttempt(params); return err }
			remove := func() error { _, err := DeleteTaskPluginVersion(row.Key, row.Version); return err }
			first, second := admit, remove
			if !admissionFirst {
				first, second = remove, admit
			}

			// Hold the first operation after taking its DB lock. Let the second
			// reach the write reservation before allowing the first to commit.
			locked, competing, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var inspected atomic.Bool
			var reservations atomic.Int32
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:version_locked", func(tx *gorm.DB) {
				if tx.Statement.Table == "task_plugins" && inspected.CompareAndSwap(false, true) {
					close(locked)
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
			}))
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:version_competing", func(tx *gorm.DB) {
				if tx.Statement.Table == "task_plugins" && reservations.Add(1) == 2 {
					close(competing)
				}
			}))
			firstResult, secondResult := make(chan error, 1), make(chan error, 1)
			go func() { firstResult <- first() }()
			select {
			case <-locked:
			case <-ctx.Done():
				t.Fatal("first operation did not acquire version lock")
			}
			go func() { secondResult <- second() }()
			select {
			case <-competing:
			case <-ctx.Done():
				t.Fatal("second operation did not reach version lock")
			}
			close(release)
			select {
			case err := <-firstResult:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("first operation stalled")
			}
			select {
			case err := <-secondResult:
				if admissionFirst {
					var inUse *SeedancePluginInUseError
					require.ErrorAs(t, err, &inUse)
					assert.Len(t, inUse.Attempts, 1)
				} else {
					require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				}
			case <-ctx.Done():
				t.Fatal("second operation stalled")
			}
			var count int64
			require.NoError(t, DB.Model(&TaskCreateAttempt{}).Count(&count).Error)
			if admissionFirst {
				assert.EqualValues(t, 1, count)
			} else {
				assert.Zero(t, count)
			}
		})
	}
}

func TestSeedancePluginTerminalDependencies(t *testing.T) {
	setupSeedancePluginUsageTest(t)
	for _, status := range TerminalTaskStatuses() {
		for _, billing := range []string{"", "settled", "pending", "failed", "debt", "awaiting_usage"} {
			task := seedancePinnedTask(string(status)+billing, string(status), billing, "v1", "62")
			require.NoError(t, DB.Create(task).Error)
			tasks, _, err := GetSeedancePluginExecutionUsage("seedance-link", "v1")
			require.NoError(t, err)
			unsettled := billing != "" && billing != "settled"
			if unsettled || status == TaskStatusSuccess {
				assert.Len(t, tasks, 1)
			} else {
				assert.Empty(t, tasks)
			}
			require.NoError(t, DB.Delete(task).Error)
		}
	}
	unknown := seedancePinnedTask("unknown", "UNKNOWN", "settled", "v1", "62")
	deleted := seedancePinnedTask("deleted", "SUCCESS", "settled", "v1", "62")
	deleted.ClientDeletedAt = 1
	for _, task := range []*Task{unknown, deleted} {
		require.NoError(t, DB.Create(task).Error)
	}
	tasks, _, err := GetSeedancePluginExecutionUsage("seedance-link", "v1")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "unknown", tasks[0].TaskId)
}
