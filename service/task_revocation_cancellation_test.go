package service

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestedCancellationCallsProviderOnce(t *testing.T) {
	truncate(t)
	original := CancelQueuedVideoTaskFunc
	calls := map[string]int{}
	CancelQueuedVideoTaskFunc = func(
		_ context.Context,
		task *model.Task,
		_ *model.Channel,
	) (bool, error) {
		calls[task.TaskID]++
		if task.TaskID == "revoked-cancel-unknown" {
			return false, errors.New("ambiguous provider cancellation")
		}
		return false, nil
	}
	t.Cleanup(func() { CancelQueuedVideoTaskFunc = original })

	for _, task := range []model.Task{
		revocationCancellationTask("revoked-cancel-confirmed"),
		revocationCancellationTask("revoked-cancel-unknown"),
	} {
		require.NoError(t, model.DB.Create(&task).Error)
	}

	assert.Equal(t, 2, ReconcileRequestedTaskCancellations(context.Background(), 10))
	assert.Zero(t, ReconcileRequestedTaskCancellations(context.Background(), 10))

	var confirmed, unknown model.Task
	require.NoError(t, model.DB.First(&confirmed, "task_id = ?", "revoked-cancel-confirmed").Error)
	require.NoError(t, model.DB.First(&unknown, "task_id = ?", "revoked-cancel-unknown").Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusCancelled), confirmed.Status)
	assert.Equal(t, model.TaskCancellationStateConfirmed, confirmed.CancellationState)
	assert.Equal(t, model.TaskStatus(model.TaskStatusQueued), unknown.Status)
	assert.Equal(t, model.TaskCancellationStateUnknown, unknown.CancellationState)
	assert.Equal(t, 1, calls["revoked-cancel-confirmed"])
	assert.Equal(t, 1, calls["revoked-cancel-unknown"])
}

func revocationCancellationTask(taskID string) model.Task {
	return model.Task{
		TaskID:            taskID,
		Platform:          constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		UserId:            9801,
		ChannelId:         9802,
		Status:            model.TaskStatusQueued,
		ClientProtocol:    model.TaskClientProtocolModelArkV3,
		CancellationState: model.TaskCancellationStateRequested,
		PrivateData: model.TaskPrivateData{Key: "frozen-key", UpstreamTaskID: "upstream-" + taskID,
			VideoUpstreamQueryBaseURL: "https://provider.example"},
	}
}
