package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelArkListEnforcesProtocolSevenDayWindowAndOfficialFilters(t *testing.T) {
	truncateTables(t)
	const userID = 981
	const appID = 1981
	const now int64 = 1_800_000_000
	statuses := []TaskStatus{
		TaskStatusQueued,
		TaskStatusInProgress,
		TaskStatusSuccess,
		TaskStatusFailure,
		TaskStatusCancelled,
		TaskStatusExpired,
	}
	for i, status := range statuses {
		serviceTier := "default"
		if i == 0 {
			serviceTier = ""
		}
		task := Task{
			TaskID:         "modelark-" + string(rune('a'+i)),
			UserId:         userID,
			AppID:          appID,
			ClientProtocol: TaskClientProtocolModelArkV3,
			Status:         status,
			CreatedAt:      now - int64(i+1),
			UpdatedAt:      now - int64(i+1),
			Properties:     Properties{OriginModelName: "seedance-model"},
			PrivateData: TaskPrivateData{ClientRequest: &TaskClientRequestSnapshot{
				ServiceTier: serviceTier,
			}},
		}
		require.NoError(t, DB.Create(&task).Error)
	}
	excluded := []Task{
		{TaskID: "too-old", UserId: userID, AppID: appID, ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusSuccess, CreatedAt: now - ModelArkTaskListWindowSeconds - 1},
		{TaskID: "future", UserId: userID, AppID: appID, ClientProtocol: TaskClientProtocolModelArkV3, Status: TaskStatusSuccess, CreatedAt: now},
		{TaskID: "kling", UserId: userID, AppID: appID, ClientProtocol: TaskClientProtocolKlingV1, Status: TaskStatusSuccess, CreatedAt: now - 1},
		{TaskID: "deleted", UserId: userID, AppID: appID, ClientProtocol: TaskClientProtocolModelArkV3, ClientDeletedAt: now - 1, Status: TaskStatusSuccess, CreatedAt: now - 1},
	}
	for i := range excluded {
		require.NoError(t, DB.Create(&excluded[i]).Error)
	}

	tasks, total, err := ListModelArkVideoTasks(userID, appID, ModelArkTaskListFilter{
		Model: "seedance-model", ServiceTier: "default", PageNum: 1, PageSize: 500, Now: now,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(len(statuses)), total)
	require.Len(t, tasks, len(statuses))

	for _, status := range []string{"queued", "running", "succeeded", "failed", "cancelled", "expired"} {
		internal, valid := ModelArkTaskStatuses(status)
		require.True(t, valid)
		filtered, filteredTotal, err := ListModelArkVideoTasks(userID, appID, ModelArkTaskListFilter{
			Statuses: internal, Model: "seedance-model", ServiceTier: "default", PageNum: 1, PageSize: 10, Now: now,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(1), filteredTotal, status)
		require.Len(t, filtered, 1, status)
		assert.Equal(t, status, filtered[0].Status.ToModelArkVideoStatus())
	}
}

func TestModelArkProjectionKeepsOfficialFieldsButHidesProviderURL(t *testing.T) {
	generateAudio := false
	draft := true
	priority := 7
	task := Task{
		TaskID:      "modelark-public-id",
		Status:      TaskStatusSuccess,
		Properties:  Properties{OriginModelName: "seedance-model"},
		PrivateData: TaskPrivateData{ClientRequest: &TaskClientRequestSnapshot{ServiceTier: "flex"}},
		Data: []byte(`{
			"content":{"video_url":"https://provider.example/signed-secret","last_frame_url":"https://provider.example/last-frame-secret"},
			"seed":42,
			"resolution":"1080p",
			"frames":121,
			"framespersecond":24,
			"generate_audio":false,
			"draft":true,
			"draft_task_id":"draft-1",
			"safety_identifier":"end-user",
			"priority":7,
			"usage":{"completion_tokens":10,"total_tokens":12,"tool_usage":{"web_search":1}}
		}`),
	}

	projected := task.ToModelArkVideoTask()

	require.NotNil(t, projected.Content)
	assert.Equal(t, "/v1/videos/modelark-public-id/content", projected.Content.VideoURL)
	assert.Equal(t, "/v1/videos/modelark-public-id/content?part=last_frame", projected.Content.LastFrameURL)
	assert.NotContains(t, projected.Content.VideoURL, "provider.example")
	assert.Equal(t, int64(42), projected.Seed)
	assert.Equal(t, 121, projected.Frames)
	assert.Equal(t, &generateAudio, projected.GenerateAudio)
	assert.Equal(t, &draft, projected.Draft)
	assert.Equal(t, &priority, projected.Priority)
	require.NotNil(t, projected.Usage)
	require.NotNil(t, projected.Usage.ToolUsage)
	assert.Equal(t, 1, projected.Usage.ToolUsage.WebSearch)
}

func TestModelArkProviderContractFailureProjectsAsFailed(t *testing.T) {
	task := Task{
		TaskID:     "modelark-contract-failure",
		Status:     TaskStatusProviderContractFailure,
		Properties: Properties{OriginModelName: "seedance-model"},
	}

	projected := task.ToModelArkVideoTask()

	assert.Equal(t, "failed", projected.Status)
	require.NotNil(t, projected.Error)
	assert.Equal(t, "provider_contract_failure", projected.Error.Code)
	statuses, valid := ModelArkTaskStatuses("failed")
	require.True(t, valid)
	assert.Contains(t, statuses, TaskStatusProviderContractFailure)
}

func TestTaskCreateIdempotencyReplayConflictExpiryAndAtomicCompletion(t *testing.T) {
	truncateTables(t)
	const userID = 982
	now := int64(1_800_000_000)

	claim, created, err := ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "key", "request-a", now+3600)
	require.NoError(t, err)
	require.True(t, created)

	replay, created, err := ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "key", "request-a", now+3600)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, claim.ID, replay.ID)

	_, _, err = ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "key", "request-b", now+3600)
	assert.ErrorIs(t, err, ErrTaskCreateIdempotencyConflict)

	task := &Task{
		TaskID: "task-idempotent", UserId: userID, ClientProtocol: TaskClientProtocolModelArkV3,
		Status: TaskStatusQueued, ChannelId: 12,
		PrivateData: TaskPrivateData{UpstreamTaskID: "upstream-idempotent"},
	}
	require.NoError(t, RecordTaskCreateUpstreamSuccess(claim.ID, task))
	require.NoError(t, DB.First(claim, "id = ?", claim.ID).Error)
	assert.Equal(t, TaskCreateIdempotencyUpstreamSucceeded, claim.Status)
	recovered, err := RecoverTaskCreateIdempotency(claim.ID)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	assert.Equal(t, task.TaskID, recovered.TaskID)
	assert.Equal(t, task.PrivateData.UpstreamTaskID, recovered.PrivateData.UpstreamTaskID)
	require.NoError(t, DB.First(claim, "id = ?", claim.ID).Error)
	assert.Equal(t, TaskCreateIdempotencyComplete, claim.Status)
	assert.Equal(t, task.TaskID, claim.TaskID)
	assert.Empty(t, claim.RecoverySnapshot)

	expired := &TaskCreateIdempotency{
		UserID: userID, Protocol: TaskClientProtocolModelArkV3, KeyHash: "expired-key",
		RequestHash: "old", Status: TaskCreateIdempotencyUnknown, ExpiresAt: 1,
	}
	require.NoError(t, DB.Create(expired).Error)
	_, created, err = ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "expired-key", "new", now+3600)
	assert.ErrorIs(t, err, ErrTaskCreateIdempotencyConflict)
	assert.False(t, created)
	reused, created, err := ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "expired-key", "old", now+3600)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, expired.ID, reused.ID)
	assert.Equal(t, TaskCreateIdempotencyUnknown, reused.Status)

	unknown, created, err := ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "unknown-key", "same-request", now+3600)
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, MarkTaskCreateIdempotencyUnknown(unknown.ID))
	unknown, created, err = ClaimTaskCreateIdempotency(userID, TaskClientProtocolModelArkV3, "unknown-key", "same-request", now+3600)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, TaskCreateIdempotencyUnknown, unknown.Status)
}

func TestTaskCancellationJournalSerializesRequestsAndTerminalTransition(t *testing.T) {
	truncateTables(t)
	task := Task{
		TaskID:         "cancel-journal",
		UserId:         984,
		AppID:          1984,
		ClientProtocol: TaskClientProtocolModelArkV3,
		Status:         TaskStatusQueued,
	}
	require.NoError(t, DB.Create(&task).Error)

	first, err := BeginTaskCancellation(task.UserId, task.AppID, task.TaskID, task.ClientProtocol)
	require.NoError(t, err)
	require.True(t, first.ShouldCall)
	assert.False(t, first.AlreadyPending)

	second, err := BeginTaskCancellation(task.UserId, task.AppID, task.TaskID, task.ClientProtocol)
	require.NoError(t, err)
	assert.False(t, second.ShouldCall)
	assert.True(t, second.AlreadyPending)

	cancelled, won, err := CompleteTaskCancellation(task.ID, true, false, "")
	require.NoError(t, err)
	assert.True(t, won)
	assert.Equal(t, TaskStatus(TaskStatusCancelled), cancelled.Status)
	assert.Equal(t, TaskCancellationStateConfirmed, cancelled.CancellationState)

	cancelled, won, err = CompleteTaskCancellation(task.ID, true, false, "")
	require.NoError(t, err)
	assert.False(t, won)
	assert.Equal(t, TaskStatus(TaskStatusCancelled), cancelled.Status)
}

func TestConfirmedCancellationWinsQueuedToRunningRace(t *testing.T) {
	truncateTables(t)
	task := &Task{
		TaskID:            "task-cancellation-running-race",
		UserId:            303,
		ClientProtocol:    TaskClientProtocolModelArkV3,
		Status:            TaskStatusInProgress,
		CancellationState: TaskCancellationStateUnknown,
	}
	require.NoError(t, DB.Create(task).Error)

	cancelled, won, err := CompleteTaskCancellation(task.ID, true, false, "")

	require.NoError(t, err)
	assert.True(t, won)
	assert.Equal(t, TaskStatus(TaskStatusCancelled), cancelled.Status)
	assert.Equal(t, TaskCancellationStateConfirmed, cancelled.CancellationState)
}
