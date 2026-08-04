package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskOperationsAreIsolatedByApplication(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	tasks := []Task{
		{TaskID: "task-app-a", UserId: 501, AppID: 1001, ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusQueued, CreatedAt: now, UpdatedAt: now},
		{TaskID: "task-app-b", UserId: 501, AppID: 1002, ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusQueued, CreatedAt: now - 1, UpdatedAt: now - 1},
	}
	require.NoError(t, DB.Create(&tasks).Error)

	task, visible, err := GetVideoTaskForProtocol(501, 1001, "task-app-a", TaskClientProtocolModelArkV3, false)
	require.NoError(t, err)
	require.True(t, visible)
	assert.Equal(t, 1001, task.AppID)
	_, visible, err = GetVideoTaskForProtocol(501, 1002, "task-app-a", TaskClientProtocolModelArkV3, false)
	require.NoError(t, err)
	assert.False(t, visible)
	_, visible, err = GetByTaskIDForApp(501, 1002, "task-app-a")
	require.NoError(t, err)
	assert.False(t, visible)
	task, visible, err = GetByTaskIDForApp(501, 1001, "task-app-a")
	require.NoError(t, err)
	require.True(t, visible)
	assert.Equal(t, "task-app-a", task.TaskID)

	listed, total, err := ListModelArkVideoTasks(501, 1001, ModelArkTaskListFilter{Now: now + 1})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, listed, 1)
	assert.Equal(t, "task-app-a", listed[0].TaskID)

	deleted, err := MarkVideoTaskClientDeleted(501, 1002, "task-app-a", TaskClientProtocolModelArkV3)
	require.NoError(t, err)
	assert.False(t, deleted)
	_, err = BeginTaskCancellation(501, 1002, "task-app-a", TaskClientProtocolModelArkV3)
	require.Error(t, err)
}
