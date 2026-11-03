package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceTaskLinksRespectQueryFiltersAndClosedWindows(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	runEvidenceTaskLinksContract(t)
}

func runEvidenceTaskLinksContract(t *testing.T) {
	t.Helper()
	index := &TaskRequestEvidence{RequestID: "batch-request", Kind: "task_polling", CreatedAt: common.GetTimestamp()}
	require.NoError(t, CreateTaskRequestEvidence(index))
	links := []EvidenceTaskLink{{TaskID: "task-one", UserID: 7, AppID: 3}, {TaskID: "task-one", UserID: 7, AppID: 3}, {TaskID: "task-two", UserID: 7, AppID: 3}}
	require.NoError(t, AttachEvidenceTasks(index.Id, links, 2*86400))
	require.NoError(t, DB.Model(index).Update("task_id", "task-one").Error)
	for _, taskID := range []string{"task-one", "task-two"} {
		rows, count, err := QueryTaskRequestEvidence(TaskRequestEvidenceQueryParams{TaskID: taskID, RequestID: index.RequestID, Kind: index.Kind})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		assert.Equal(t, index.Id, rows[0].Id)
	}
	_, count, err := QueryTaskRequestEvidence(TaskRequestEvidenceQueryParams{TaskID: "task-one", RequestID: "other-request"})
	require.NoError(t, err)
	assert.Zero(t, count)
	require.NoError(t, RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
	assert.ErrorIs(t, AttachEvidenceTasks(index.Id, links, 2*86400), ErrTaskRequestEvidenceBodyClosed)
	require.NoError(t, DB.Model(index).Update("body_expired", true).Error)
	deleted, err := DeleteExpiredEvidenceIndex(index.Id, common.GetTimestamp())
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	assert.ErrorIs(t, AttachEvidenceTasks(index.Id, links, 2*86400), ErrTaskRequestEvidenceBodyClosed)
	require.NoError(t, DB.Model(&EvidenceTaskLink{}).Count(&count).Error)
	assert.Zero(t, count)
}
