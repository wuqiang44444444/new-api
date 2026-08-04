package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageTaskProjectionHidesProviderStateAndReturnsAllURLs(t *testing.T) {
	task := &Task{
		TaskID:         "task_public",
		CreatedAt:      100,
		FinishTime:     120,
		Status:         TaskStatusSuccess,
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: TaskClientProtocolOpenAIImages,
		Properties:     Properties{OriginModelName: "seedream-5-0-260128"},
		PrivateData: TaskPrivateData{
			Key:            "secret",
			UpstreamTaskID: "provider-task",
			MediaImage: &TaskMediaImagePrivateData{
				ResultURLs: []string{"https://cdn.example/one.png", "https://cdn.example/two.png"},
			},
		},
	}

	projected := ProjectOpenAIImageTask(task)
	assert.Equal(t, "task_public", projected.ID)
	assert.Equal(t, "completed", projected.Status)
	require.NotNil(t, projected.Result)
	require.Len(t, projected.Result.Data, 2)
	assert.Equal(t, "https://cdn.example/one.png", projected.Result.Data[0].Url)
	assert.NotContains(t, projected.ID, "provider-task")
}

func TestOpenAIImageTaskProjectionSanitizesFailure(t *testing.T) {
	projected := ProjectOpenAIImageTask(&Task{
		TaskID:     "task_public",
		Status:     TaskStatusFailure,
		FailReason: "authorization failed at https://provider.example?api_key=secret",
	})
	require.NotNil(t, projected.Error)
	assert.Equal(t, "Image generation failed", projected.Error.Message)
}

func TestGetTaskForProtocolEnforcesOwnerAndProtocol(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID:         "task_image",
		UserId:         701,
		Status:         TaskStatusQueued,
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: TaskClientProtocolOpenAIImages,
	}
	require.NoError(t, DB.Create(task).Error)

	got, exists, err := GetTaskForProtocol(701, task.TaskID, TaskClientProtocolOpenAIImages, false)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, task.TaskID, got.TaskID)

	_, exists, err = GetTaskForProtocol(702, task.TaskID, TaskClientProtocolOpenAIImages, false)
	require.NoError(t, err)
	assert.False(t, exists)

	_, exists, err = GetTaskForProtocol(701, task.TaskID, TaskClientProtocolModelArkV3, false)
	require.NoError(t, err)
	assert.False(t, exists)
}
