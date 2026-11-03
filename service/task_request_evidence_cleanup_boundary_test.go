package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceCleanupBeforeStagingPreventsLateFile(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "deleted"}[deleted], func(t *testing.T) {
			setupEvidenceTestEnv(t)
			index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
			require.NoError(t, model.CreateTaskRequestEvidence(index))
			event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id}
			require.NoError(t, model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()))
			require.NoError(t, model.RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
			if deleted {
				require.NoError(t, model.DB.Delete(event).Error)
				require.NoError(t, model.DB.Delete(index).Error)
			}
			store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
			object, err := store.beginStream(EvidenceObjectKey(index.Id, event.Id), index.Id)
			require.ErrorIs(t, err, model.ErrTaskRequestEvidenceBodyClosed)
			assert.Nil(t, object)
			entries, err := os.ReadDir(store.baseDir)
			require.NoError(t, err)
			assert.Empty(t, entries, "late staging must not recreate a cleaned namespace")
		})
	}
}

func TestEvidenceCleanupBetweenStreamingAndPublicationPreventsLateFile(t *testing.T) {
	setupEvidenceTestEnv(t)
	index := &model.TaskRequestEvidence{CreatedAt: common.GetTimestamp()}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id}
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(event, EvidenceRetentionSeconds()))
	store := GetTaskRequestEvidenceStore().(*localEncryptedEvidenceStore)
	key := EvidenceObjectKey(index.Id, event.Id)
	object, err := store.beginStream(key, index.Id)
	require.NoError(t, err)
	_, err = object.Write([]byte("original bytes"))
	require.NoError(t, err)
	_, err = store.openStream(key)
	require.ErrorIs(t, err, ErrTaskRequestEvidenceUnavailable, "unpublished ciphertext is not readable")
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(index.Id, common.GetTimestamp()))
	require.ErrorIs(t, object.Close(), model.ErrTaskRequestEvidenceBodyClosed)
	_, err = os.Stat(filepath.Join(store.baseDir, key))
	assert.True(t, os.IsNotExist(err))
	entries, err := os.ReadDir(filepath.Join(store.baseDir, filepath.Dir(key)))
	require.NoError(t, err)
	assert.Empty(t, entries, "late worker leaves no ciphertext staging file")
}
func TestEvidenceNewWindowDoesNotReopenClosedTaskWindow(t *testing.T) {
	setupEvidenceTestEnv(t)
	task := &model.Task{TaskID: "same-task", UserId: 7, AppID: 3, CreatedAt: common.GetTimestamp() - 3*86400}
	first := newAPIRequestEvidence(evidenceIndexForTask(task))
	first.newRecord("polling", EvidenceObjectMeta{}, false).close()
	first.Finish(false)
	old := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.Len(t, old, 1)
	require.NoError(t, model.RequestTaskRequestEvidenceCleanup(old[0].Id, common.GetTimestamp()))
	next := newAPIRequestEvidence(evidenceIndexForTask(task))
	next.newRecord("polling", EvidenceObjectMeta{}, false).close()
	next.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{TaskID: task.TaskID})
	require.Len(t, indexes, 2)
	assert.NotEqual(t, indexes[0].Id, indexes[1].Id)
	assert.GreaterOrEqual(t, indexes[0].CreatedAt, common.GetTimestamp()-1)
	rows := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, rows, 1)
	_, status := ReadEvidenceEventContent(rows[0], false)
	assert.Equal(t, "available", status)
}
