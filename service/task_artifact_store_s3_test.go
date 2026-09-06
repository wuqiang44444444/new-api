package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestS3Store(t *testing.T) *s3ArtifactStore {
	t.Helper()
	store, err := NewS3ArtifactStore(system_setting.TaskArtifactStoreConfig{
		Mode:       system_setting.TaskArtifactStoreModeS3,
		S3Endpoint: "https://objects.example.com", S3Bucket: "task-artifacts",
		S3Region: "us-east-1", S3AccessKey: "ak", S3SecretKey: "sk", S3PresignTTLSeconds: 900,
	})
	require.NoError(t, err)
	return store.(*s3ArtifactStore)
}

// 评审 S5：Resolve 只对有持久化登记事实的图片产物返回引用；既有插件产物
// （从未入库）必须返回 nil，保持原 Provider 下载路径不被劫持。
func TestS3ResolveRequiresPersistedImageArtifact(t *testing.T) {
	store := newTestS3Store(t)

	pluginTask := &model.Task{TaskID: "task_plugin", Status: model.TaskStatusSuccess}
	ref, err := store.Resolve(pluginTask, "video")
	require.NoError(t, err)
	assert.Nil(t, ref, "plugin artifacts without persistence facts must not resolve")

	imageTask := &model.Task{
		TaskID: "task_img", Status: model.TaskStatusSuccess,
		ClientProtocol: model.TaskClientProtocolImageOpenAIV1,
	}
	imageTask.PrivateData.ImageTask = &model.TaskImageExecutionData{
		Artifacts: []model.TaskImageArtifact{{
			ObjectKey: model.ImageTaskArtifactObjectKey("task_img", "result-0"),
			MimeType:  "image/png",
		}},
	}
	ref, err = store.Resolve(imageTask, "result-0")
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, "images/tasks/task_img/result-0", ref.ObjectKey)
}

func TestBuildImageTaskObjectKeyIsDeterministic(t *testing.T) {
	first, err := BuildImageTaskObjectKey("task_x", "result-0")
	require.NoError(t, err)
	second, err := BuildImageTaskObjectKey("task_x", "result-0")
	require.NoError(t, err)
	assert.Equal(t, first, second, "no random segment: crash recovery can re-register by key")
	assert.Equal(t, "images/tasks/task_x/result-0", first)

	_, err = BuildImageTaskObjectKey("../escape", "x")
	assert.Error(t, err)
}

var _ = common.GetTimestamp
