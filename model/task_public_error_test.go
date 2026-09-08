package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicVideoFailureSurvivesPersistenceAndProtectsHistoricalRows(t *testing.T) {
	truncateTables(t)
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		task := Task{TaskID: GenerateTaskID(), Status: TaskStatusInProgress, Quota: 700}
		require.NoError(t, DB.Create(&task).Error)
		body, err := common.Marshal(map[string]any{"status": "failed", "error": map[string]any{"code": "ContentPolicyViolation", "message": message}})
		require.NoError(t, err)
		task.Status, task.FailReason, task.Data = TaskStatusFailure, message, body
		won, err := task.UpdateWithStatus(TaskStatusInProgress)
		require.NoError(t, err)
		require.True(t, won)
		var stored Task
		require.NoError(t, DB.First(&stored, task.ID).Error)
		public := stored.ToModelArkVideoTask()
		require.NotNil(t, public.Error)
		assert.Equal(t, message, public.Error.Message)
		assert.Equal(t, "ContentPolicyViolation", public.Error.Code)
		assert.Equal(t, message, stored.ToOpenAIVideo().Error.Message)
		assert.Equal(t, 700, stored.Quota, "error normalization must not adjust funding")
		stale := stored
		stale.FailReason = "network timeout"
		won, err = stale.UpdateWithStatus(TaskStatusInProgress)
		require.NoError(t, err)
		assert.False(t, won)
		require.NoError(t, DB.First(&stored, task.ID).Error)
		assert.Equal(t, message, stored.PublicFailReason())
		stored.FailReason = "poll failed: unrecognized; body={\"secret\":\"hidden\"}"
		assert.Equal(t, "generation_failed", stored.PublicVideoFailure().Code, "old Data must not supply the code for a different failure")
		assert.NotContains(t, stored.PublicVideoFailure().Message, "hidden")
	}
	task := Task{Status: TaskStatusFailure, FailReason: "FunCloud rejected model upstream-private, channel ID: 72", Properties: Properties{OriginModelName: "public", UpstreamModelName: "upstream-private"}}
	assert.Equal(t, "video service rejected model requested model,", task.PublicFailReason())
	for _, status := range []TaskStatus{TaskStatusCancelled, TaskStatusExpired, TaskStatusProviderContractFailure} {
		task.Status = status
		assert.NotContains(t, task.ToModelArkVideoTask().Error.Message, "upstream-private")
		assert.NotEqual(t, task.PublicFailReason(), task.ToModelArkVideoTask().Error.Message)
	}
}

func TestPublicVideoErrorsRespectSuccessAndClientModel(t *testing.T) {
	legacy := Task{Status: TaskStatusSuccess, FailReason: "https://media.example/video?signature=old"}
	assert.Empty(t, legacy.PublicFailReason())
	assert.Nil(t, legacy.ToOpenAIVideo().Error)
	assert.Equal(t, legacy.FailReason, legacy.PublicVideoResultURL())
	legacy.Status = TaskStatusFailure
	assert.Empty(t, legacy.PublicVideoResultURL())
	legacy.Properties = Properties{OriginModelName: "openai/sora", UpstreamModelName: "provider-private"}
	legacy.FailReason = "model openai/sora (provider-private) rejected: invalid duration"
	assert.Equal(t, "model openai/sora (requested model) rejected: invalid duration", legacy.PublicFailReason())
}

func TestTaskCASDoesNotRewriteNonVideoDiagnostics(t *testing.T) {
	truncateTables(t)
	task := Task{TaskID: GenerateTaskID(), Platform: "suno", Status: TaskStatusInProgress}
	require.NoError(t, DB.Create(&task).Error)
	task.FailReason = "decode response body: unexpected end of JSON input"
	won, err := task.UpdateWithStatus(TaskStatusInProgress)
	require.NoError(t, err)
	require.True(t, won)
	var stored Task
	require.NoError(t, DB.First(&stored, task.ID).Error)
	assert.Equal(t, task.FailReason, stored.FailReason)
	task.FailReason = "FunCloud internal diagnostic; body={\"details\":1}"
	won, err = task.UpdateWithStatus(TaskStatusFailure)
	require.NoError(t, err)
	assert.False(t, won)
	assert.Equal(t, "FunCloud internal diagnostic; body={\"details\":1}", task.FailReason)
}
