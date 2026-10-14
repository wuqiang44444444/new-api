package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvidenceDatabaseFailureResumesAfterBodyCleanup(t *testing.T) {
	store, dir := setupEvidenceCleanupEnv(t)
	index := seedExpiredEvidence(t, store, dir, 8*86400)
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register("evidence-test-delete-failure", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "task_request_evidence_events" {
			tx.AddError(errors.New("test database deletion unavailable"))
		}
	}))
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	saved, exists, err := model.GetTaskRequestEvidenceById(index.Id)
	require.NoError(t, err)
	require.True(t, exists)
	assert.True(t, saved.BodyExpired)
	assert.Positive(t, saved.CleanupRequestedAt)
	events, err := model.ListTaskRequestEvidenceEvents(index.Id)
	require.NoError(t, err)
	require.Len(t, events, 2)
	for _, event := range events {
		_, err := os.Stat(filepath.Join(dir, service.EvidenceObjectKey(index.Id, event.Id)))
		assert.True(t, os.IsNotExist(err))
	}
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	require.NoError(t, model.DB.Callback().Delete().Remove("evidence-test-delete-failure"))
	task, runner = createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	_, exists, err = model.GetTaskRequestEvidenceById(index.Id)
	require.NoError(t, err)
	assert.False(t, exists)
	events, err = model.ListTaskRequestEvidenceEvents(index.Id)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestEvidenceCleanupReclaimsLegacyCompletedRowsAndOrphanObjects(t *testing.T) {
	store, dir := setupEvidenceCleanupEnv(t)
	index := seedExpiredEvidence(t, store, dir, 8*86400)
	require.NoError(t, model.DB.Model(index).Updates(map[string]any{"body_expired": true, "cleanup_requested_at": 0}).Error)
	orphan := &model.TaskRequestEvidenceEvent{EvidenceId: 99999, CreatedAt: common.GetTimestamp() - 8*86400}
	require.NoError(t, model.DB.Create(orphan).Error)
	require.NoError(t, store.Put(service.EvidenceObjectKey(orphan.EvidenceId, orphan.Id), []byte("orphan")))
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidence{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceEvent{}).Count(&count).Error)
	assert.Zero(t, count)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestManualEvidenceCleanupRemovesDetailsButKeepsFiniteClosureIndex(t *testing.T) {
	store, dir := setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	index := seedExpiredEvidence(t, store, dir, 86400)
	task, created, err := service.StartEvidenceCleanupTask(now, 7)
	require.NoError(t, err)
	require.True(t, created)
	_, claimed, err := model.ClaimSystemTask(task.ID, task.Type, "manual-database", now+3600)
	require.NoError(t, err)
	require.True(t, claimed)
	evidenceRetentionHandler{}.Run(context.Background(), task, "manual-database")
	saved, exists, err := model.GetTaskRequestEvidenceById(index.Id)
	require.NoError(t, err)
	require.True(t, exists)
	assert.True(t, saved.BodyExpired)
	events, err := model.ListTaskRequestEvidenceEvents(index.Id)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.ErrorIs(t, model.CreateTaskRequestEvidenceEvent(&model.TaskRequestEvidenceEvent{EvidenceId: index.Id}), model.ErrTaskRequestEvidenceBodyClosed)
	require.NoError(t, model.DB.Model(saved).UpdateColumn("created_at", now-8*86400).Error)
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	_, exists, err = model.GetTaskRequestEvidenceById(index.Id)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestUnavailableEvidenceStoreStillReclaimsIndependentAuditHistory(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	audit := &model.TaskRequestEvidenceAccessLog{EvidenceId: 9999, CreatedAt: now - 8*86400}
	require.NoError(t, model.DB.Create(audit).Error)
	old := &model.SystemTask{TaskID: "old-cleanup", Type: model.SystemTaskTypeTaskRequestEvidenceRetention, Status: model.SystemTaskStatusSucceeded, CreatedAt: now - 31*86400, UpdatedAt: now - 31*86400}
	require.NoError(t, model.DB.Create(old).Error)
	// Invalid storage produces no handle; no production configuration is used.
	require.Error(t, service.InitTaskRequestEvidenceStore(system_setting.TaskRequestEvidenceConfig{StorageDir: t.TempDir(), EncryptionKeyHex: "invalid"}))
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	var count int64
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceAccessLog{}).Count(&count).Error)
	assert.Zero(t, count)
	assert.ErrorIs(t, model.DB.First(&model.SystemTask{}, old.ID).Error, gorm.ErrRecordNotFound)
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	var stats evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &stats))
	assert.Equal(t, "store_unavailable", stats.ErrorCategory)
	assert.EqualValues(t, 1, stats.DeletedAccessLogs)
	assert.EqualValues(t, 1, stats.DeletedHistory)
}

func TestEvidenceCleanupTraversesEventPagesBeforeDeletingIndex(t *testing.T) {
	store, dir := setupEvidenceCleanupEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp() - 8*86400}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	// One row beyond the cleanup page proves both file and database traversal.
	events := make([]model.TaskRequestEvidenceEvent, 501)
	for i := range events {
		events[i] = model.TaskRequestEvidenceEvent{EvidenceId: index.Id, Phase: model.TaskRequestEvidencePhaseUnavailable}
	}
	require.NoError(t, model.DB.CreateInBatches(&events, 100).Error)
	for _, event := range []model.TaskRequestEvidenceEvent{events[0], events[500]} {
		require.NoError(t, store.Put(service.EvidenceObjectKey(index.Id, event.Id), []byte("boundary")))
	}
	task, runner := createCleanupTask(t)
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	var stats evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &stats))
	assert.EqualValues(t, 501, stats.DeletedEvents)
	assert.EqualValues(t, 1, stats.DeletedIndexes)
	assert.EqualValues(t, 2, stats.DeletedFiles)
	assert.EqualValues(t, 499, stats.MissingFiles)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestEvidenceDatabaseCleanupPublishesProgressBeforeCompletion(t *testing.T) {
	setupEvidenceCleanupEnv(t)
	now := common.GetTimestamp()
	index := &model.TaskRequestEvidence{CreatedAt: now - 8*86400, BodyExpired: true, CleanupRequestedAt: now}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	events := make([]model.TaskRequestEvidenceEvent, 501)
	for i := range events {
		events[i].EvidenceId = index.Id
	}
	require.NoError(t, model.DB.CreateInBatches(&events, 100).Error)
	task, runner := createCleanupTask(t)
	var observed []int64
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register("evidence-progress-observer", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != "task_request_evidence_events" {
			return
		}
		var running model.SystemTask
		require.NoError(t, tx.Session(&gorm.Session{NewDB: true}).Where("task_id = ?", task.TaskID).First(&running).Error)
		assert.Equal(t, model.SystemTaskStatusRunning, running.Status)
		var state evidenceRetentionStats
		if running.State != "" {
			require.NoError(t, common.UnmarshalJsonStr(running.State, &state))
		}
		observed = append(observed, state.DeletedEvents)
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Delete().Remove("evidence-progress-observer")) })
	evidenceRetentionHandler{}.Run(context.Background(), task, runner)
	assert.Equal(t, []int64{0, 500}, observed)
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	var result evidenceRetentionStats
	require.NoError(t, common.UnmarshalJsonStr(finished.Result, &result))
	assert.EqualValues(t, 501, result.DeletedEvents)
	assert.EqualValues(t, 1, result.DeletedIndexes)
}
