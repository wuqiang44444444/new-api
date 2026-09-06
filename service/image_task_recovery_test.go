package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRecoveryWithoutProviderIDPreservesPartialResults(t *testing.T) {
	truncate(t)
	seedImageTaskUser(t, 9904, 1000)
	var missingFirst atomic.Bool
	missingFirst.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodHead, r.Method, "recovery must never regenerate or download an untrusted URL")
		if missingFirst.Load() && strings.HasSuffix(r.URL.Path, "result-0") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store, err := NewS3ArtifactStore(system_setting.TaskArtifactStoreConfig{
		Mode: system_setting.TaskArtifactStoreModeS3, S3Endpoint: server.URL, S3Bucket: "images",
		S3Region: "us-east-1", S3AccessKey: "test", S3SecretKey: "test", S3PresignTTLSeconds: 300,
	})
	require.NoError(t, err)
	previous := taskArtifactStoreRuntime.get()
	previousRevision := taskArtifactStoreRuntime.revision
	taskArtifactStoreRuntime.swap(store, "")
	t.Cleanup(func() { taskArtifactStoreRuntime.swap(previous, previousRevision) })
	task := newWorkerImageTask(9904, 100)
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	task, won, err := model.ClaimImageTask(task.TaskID, model.ImageTaskExecutionScopeGlobal(), 1, model.ImageTaskExecutionScopeChannel(task.ChannelId), 1)
	require.NoError(t, err)
	require.True(t, won)
	won, err = model.MarkImageTaskSending(task)
	require.NoError(t, err)
	require.True(t, won)
	manifest := []model.TaskImageArtifact{
		{ObjectKey: model.ImageTaskArtifactObjectKey(task.TaskID, "result-0"), MimeType: "image/png"},
		{ObjectKey: model.ImageTaskArtifactObjectKey(task.TaskID, "result-1"), MimeType: "image/png"},
	}
	won, err = model.RecordImageTaskGeneration(task, manifest, &dto.Usage{PromptTokens: 5, CompletionTokens: 10, TotalTokens: 15})
	require.NoError(t, err)
	require.True(t, won)
	won, err = model.FinishImageTaskFailure(task, model.TaskStatusReconciliationRequired, "result_store_failed")
	require.NoError(t, err)
	require.True(t, won)
	recovered := model.GetReconcilableImageTasks(10)
	require.Len(t, recovered, 1, "Google tasks must be discoverable without a Provider task ID")
	task, won, err = model.ClaimImageTaskRecovery(task.TaskID, model.ImageTaskExecutionScopeGlobal(), 1, model.ImageTaskExecutionScopeChannel(task.ChannelId), 1)
	require.NoError(t, err)
	require.True(t, won)
	result, err := RecoverImageTaskArtifacts(context.Background(), task)
	require.NoError(t, err)
	assert.Nil(t, result, "missing bytes cannot be claimed as delivered")
	persisted := reloadTask(t, task.ID)
	require.Len(t, persisted.PrivateData.ImageTask.Artifacts, 1, "the available image must remain queryable")
	assert.Equal(t, 900, getUserQuota(t, 9904), "partial delivery never triggers a refund")
	missingFirst.Store(false)
	result, err = RecoverImageTaskArtifacts(context.Background(), task)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, ImageTaskOutcomeSuccess, result.Outcome)
	require.Len(t, result.Images, 2)
	assert.Equal(t, manifest[0].ObjectKey, result.Images[0].ObjectKey)
	assert.Equal(t, manifest[1].ObjectKey, result.Images[1].ObjectKey)
	assert.Equal(t, 15, result.Usage.TotalTokens)
	won, err = model.FinishImageTaskSuccess(task, result.Images, result.Usage)
	require.NoError(t, err)
	require.True(t, won)
	assert.EqualValues(t, model.TaskStatusSuccess, reloadTask(t, task.ID).Status)
}
