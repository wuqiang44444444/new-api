package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupEvidenceLifecycleDatabase(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lifecycle.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Task{}, &TaskRequestEvidence{}, &TaskRequestEvidenceEvent{}, &TaskRequestEvidenceAccessLog{}, &SystemTask{}, &SystemTaskLock{}))
	previousDB, previousType := DB, common.MainDatabaseType()
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() { DB = previousDB; common.SetMainDatabaseType(previousType) })
}

func TestEvidenceEventReservationRejectsClosedMissingAndExpiredParent(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	now := common.GetTimestamp()
	for _, index := range []*TaskRequestEvidence{
		{CreatedAt: now, CleanupRequestedAt: now},
		{CreatedAt: now, BodyExpired: true},
		{CreatedAt: now - 8*86400},
		{Id: 9999},
	} {
		if index.Id == 0 {
			require.NoError(t, CreateTaskRequestEvidence(index))
		}
		event := &TaskRequestEvidenceEvent{EvidenceId: index.Id, Stage: TaskRequestEvidenceStageTransport}
		assert.ErrorIs(t, CreateTaskRequestEvidenceEvent(event), ErrTaskRequestEvidenceBodyClosed)
	}
	var count int64
	require.NoError(t, DB.Model(&TaskRequestEvidenceEvent{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEvidenceDatabaseDeletionContract(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	runEvidenceDatabaseDeletionContract(t)
}

// Shared with opt-in MySQL/PostgreSQL tests: observable recovery and boundaries.
func runEvidenceDatabaseDeletionContract(t *testing.T) {
	t.Helper()
	now := common.GetTimestamp()
	task := &Task{TaskID: "contract-media-task", UserId: 7, AppID: 3, CreatedAt: now - 3600, ClientProtocol: "openai_video"}
	require.NoError(t, DB.Create(task).Error)
	lateIndex := &TaskRequestEvidence{TaskID: task.TaskID, UserID: task.UserId, AppID: task.AppID, CreatedAt: now, RetainUntil: now + 7*86400}
	require.NoError(t, CreateTaskRequestEvidence(lateIndex))
	fixed, err := GetOrCreateTaskMediaEvidence(task, "video_task", now, 7*86400)
	require.NoError(t, err)
	assert.Equal(t, lateIndex.Id, fixed.Id)
	assert.Equal(t, task.CreatedAt+7*86400, fixed.RetainUntil)
	require.NoError(t, RequestTaskRequestEvidenceCleanup(fixed.Id, now))
	_, err = GetOrCreateTaskMediaEvidence(task, "video_task", now, 7*86400)
	assert.ErrorIs(t, err, ErrTaskRequestEvidenceBodyClosed)
	index := &TaskRequestEvidence{CreatedAt: now - 8*86400, BodyExpired: true, CleanupRequestedAt: now}
	require.NoError(t, CreateTaskRequestEvidence(index))
	events := []*TaskRequestEvidenceEvent{{EvidenceId: index.Id}, {EvidenceId: index.Id}, {EvidenceId: index.Id}}
	require.NoError(t, DB.Create(&events).Error) // Historical data, not new writes.
	deleted, err := DeleteClosedEvidenceEventBatch(index.Id, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	deleted, err = DeleteExpiredEvidenceIndex(index.Id, now)
	require.NoError(t, err)
	assert.Zero(t, deleted, "remaining events retain their parent")
	candidates, err := TaskRequestEvidenceDatabaseCandidates(now, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	deleted, err = DeleteClosedEvidenceEventBatch(index.Id, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	deleted, err = DeleteExpiredEvidenceIndex(index.Id, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.ErrorIs(t, CreateTaskRequestEvidenceEvent(&TaskRequestEvidenceEvent{EvidenceId: index.Id}), ErrTaskRequestEvidenceBodyClosed)
	assert.ErrorIs(t, FinishTaskRequestEvidenceEvent(events[0]), ErrTaskRequestEvidenceBodyClosed)
	// A manual early cleanup leaves a finite closure index; exact day seven stays.
	for _, age := range []int64{86400, 7 * 86400} {
		closed := &TaskRequestEvidence{CreatedAt: now - age, BodyExpired: true, CleanupRequestedAt: now}
		require.NoError(t, CreateTaskRequestEvidence(closed))
		deleted, err = DeleteExpiredEvidenceIndex(closed.Id, now)
		require.NoError(t, err)
		assert.Zero(t, deleted)
	}
	// Audit retention starts at access time and is independent of index deletion.
	audits := []*TaskRequestEvidenceAccessLog{{EvidenceId: index.Id, CreatedAt: now - 8*86400}, {EvidenceId: index.Id, CreatedAt: now - 7*86400}, {EvidenceId: index.Id, CreatedAt: now}}
	require.NoError(t, DB.Create(&audits).Error)
	deleted, err = DeleteExpiredEvidenceAccessBatch(now, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var count int64
	require.NoError(t, DB.Model(&TaskRequestEvidenceAccessLog{}).Where("evidence_id = ?", index.Id).Count(&count).Error)
	assert.EqualValues(t, 2, count)
}

func TestEvidenceHistoryDeletionPreservesActiveLatestAndOtherTypes(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	now := common.GetTimestamp()
	rows := []*SystemTask{
		{TaskID: "old", Type: SystemTaskTypeTaskRequestEvidenceRetention, Status: SystemTaskStatusSucceeded, CreatedAt: now - 31*86400, UpdatedAt: now - 31*86400},
		{TaskID: "latest", Type: SystemTaskTypeTaskRequestEvidenceRetention, Status: SystemTaskStatusFailed, CreatedAt: now - 31*86400, UpdatedAt: now - 31*86400},
		{TaskID: "active", Type: SystemTaskTypeTaskRequestEvidenceMonitor, Status: SystemTaskStatusRunning, CreatedAt: now - 31*86400, UpdatedAt: now - 31*86400},
		{TaskID: "other", Type: SystemTaskTypeAsyncTaskPoll, Status: SystemTaskStatusSucceeded, CreatedAt: now - 31*86400, UpdatedAt: now - 31*86400},
	}
	require.NoError(t, DB.Create(&rows).Error)
	deleted, err := DeleteExpiredEvidenceTaskHistoryBatch(now, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var ids []string
	require.NoError(t, DB.Model(&SystemTask{}).Order("id").Pluck("task_id", &ids).Error)
	assert.Equal(t, []string{"latest", "active", "other"}, ids)
}
