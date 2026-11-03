package service

import (
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskPressureImmediatelyClosesOnlyEvidenceOlderThanOneDay(t *testing.T) {
	setupEvidenceTestEnv(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemTask{}))
	last := evidenceDiskCleanupLast.Load()
	evidenceDiskCleanupLast.Store(0)
	t.Cleanup(func() { evidenceDiskCleanupLast.Store(last) })
	now := common.GetTimestamp()
	older := &model.TaskRequestEvidence{CreatedAt: now - 86401}
	boundary := &model.TaskRequestEvidence{CreatedAt: now - 86400}
	recent := &model.TaskRequestEvidence{CreatedAt: now - 3600}
	for _, index := range []*model.TaskRequestEvidence{older, boundary, recent} {
		require.NoError(t, model.DB.Create(index).Error)
	}
	started, err := ObserveEvidenceDiskUsage(now, 90)
	require.NoError(t, err)
	assert.False(t, started)
	assert.False(t, evidenceDiskPressure.Load())
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: "active-at-pressure"})
	record := capture.newRecord("north_receive", EvidenceObjectMeta{}, false)
	record.append(bytes.Repeat([]byte("x"), 64<<10))
	activeIndexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{RequestID: "active-at-pressure"})
	require.Len(t, activeIndexes, 1)
	activeEvents := evidenceEventRows(t, activeIndexes[0].Id)
	require.Len(t, activeEvents, 1)
	started, err = ObserveEvidenceDiskUsage(now, 90.01)
	require.NoError(t, err)
	assert.True(t, started)
	assert.True(t, evidenceDiskPressure.Load())
	record.close()
	capture.Finish(false)
	drainImageEvidence(t)
	_, bodyStatus := ReadEvidenceEventContent(activeEvents[0], false)
	assert.Equal(t, "missing", bodyStatus, "pressure aborts an existing staged object before final publication")
	task, err := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
	require.NoError(t, err)
	require.NotNil(t, task)
	var payload EvidenceCleanupPayload
	require.NoError(t, task.DecodePayload(&payload))
	assert.Equal(t, "disk_pressure", payload.Trigger)
	assert.Equal(t, now-86400, payload.TargetTimestamp)
	for _, index := range []*model.TaskRequestEvidence{older, boundary, recent} {
		require.NoError(t, model.DB.First(index, index.Id).Error)
	}
	assert.Positive(t, older.CleanupRequestedAt)
	assert.Zero(t, boundary.CleanupRequestedAt)
	assert.Zero(t, recent.CleanupRequestedAt)
	started, err = ObserveEvidenceDiskUsage(now+1, 99)
	require.NoError(t, err)
	assert.False(t, started, "no duplicate cleanup while one is active")
	assert.Nil(t, newAPIRequestEvidence(model.TaskRequestEvidence{}).newRecord("north_receive", EvidenceObjectMeta{}, false), "pressure halts new observation")
	_, err = ObserveEvidenceDiskUsage(now+2, 89)
	require.NoError(t, err)
	assert.False(t, evidenceDiskPressure.Load())
	assert.EqualValues(t, 2*86400, EvidenceRetentionSeconds())
	require.NoError(t, model.DB.First(older, older.Id).Error)
	assert.Positive(t, older.CleanupRequestedAt, "pressure recovery cannot reopen old evidence")
}
