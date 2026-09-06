package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreRuntime 在测试结束后恢复全局存储状态，避免污染同包其它用例。
func restoreRuntime(t *testing.T) {
	t.Helper()
	previousStore := taskArtifactStoreRuntime.get()
	previousRevision := taskArtifactStoreRuntime.revision
	t.Cleanup(func() {
		taskArtifactStoreRuntime.swap(previousStore, previousRevision)
	})
}

func buildRuntimeTestConfig(t *testing.T, backend string) system_setting.ObjectStorageConfig {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "runtime-test-master-key")
	previous := common.CryptoSecret
	common.CryptoSecret = "runtime-test-master-key"
	t.Cleanup(func() { common.CryptoSecret = previous })

	// Azure Shared Key 要求凭据本身是合法 Base64（SDK 会解码为字节密钥）。
	ciphertext, err := common.EncryptObjectStorageCredential("dGVzdC1zZWNyZXQ=")
	require.NoError(t, err)
	// Azure Storage Account 只允许小写字母与数字；S3 Access Key ID 允许更多字符。
	accountName := "access-key-id"
	endpoint := "https://storage.example.com"
	if backend == system_setting.ObjectStorageBackendAzureBlob {
		accountName = "storeaccount"
		endpoint = "https://storeaccount.blob.core.windows.net"
	}
	return system_setting.ObjectStorageConfig{
		Backend:              backend,
		Endpoint:             endpoint,
		Bucket:               "task-artifacts",
		Region:               "us-east-1",
		AccountName:          accountName,
		CredentialCiphertext: ciphertext,
		Revision:             "rev-1",
	}
}

func TestApplyTaskArtifactStoreSetting(t *testing.T) {
	restoreRuntime(t)

	t.Run("empty setting keeps storage disabled", func(t *testing.T) {
		applyTaskArtifactStoreSetting("")
		assert.False(t, GetTaskArtifactStore().Enabled())
	})

	t.Run("incomplete configuration never activates", func(t *testing.T) {
		config := buildRuntimeTestConfig(t, system_setting.ObjectStorageBackendS3)
		config.Bucket = ""
		data, err := common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		assert.False(t, GetTaskArtifactStore().Enabled())
	})

	t.Run("invalid JSON fails closed", func(t *testing.T) {
		applyTaskArtifactStoreSetting("{not-json")
		assert.False(t, GetTaskArtifactStore().Enabled())
	})

	t.Run("valid s3 configuration activates and revision refresh is idempotent", func(t *testing.T) {
		config := buildRuntimeTestConfig(t, system_setting.ObjectStorageBackendS3)
		data, err := common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		require.True(t, GetTaskArtifactStore().Enabled())

		first := GetTaskArtifactStore()
		// 同一 revision 重复装载（多节点周期同步）不得重建实例。
		applyTaskArtifactStoreSetting(string(data))
		assert.Same(t, first, GetTaskArtifactStore())

		// revision 变化触发重建。
		config.Revision = "rev-2"
		data, err = common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		assert.NotSame(t, first, GetTaskArtifactStore())
	})

	t.Run("azure blob configuration activates azure store", func(t *testing.T) {
		config := buildRuntimeTestConfig(t, system_setting.ObjectStorageBackendAzureBlob)
		data, err := common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		require.True(t, GetTaskArtifactStore().Enabled())
		require.IsType(t, &azureBlobArtifactStore{}, GetTaskArtifactStore())
	})

	t.Run("explicit upstream disables storage", func(t *testing.T) {
		config := buildRuntimeTestConfig(t, system_setting.ObjectStorageBackendS3)
		data, err := common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		require.True(t, GetTaskArtifactStore().Enabled())

		applyTaskArtifactStoreSetting(`{"backend":"upstream","revision":"rev-off"}`)
		assert.False(t, GetTaskArtifactStore().Enabled())
	})

	t.Run("undecryptable credential fails closed", func(t *testing.T) {
		config := buildRuntimeTestConfig(t, system_setting.ObjectStorageBackendS3)
		config.CredentialCiphertext = "objstore.v1.bm90LXZhbGlk"
		data, err := common.Marshal(config)
		require.NoError(t, err)
		applyTaskArtifactStoreSetting(string(data))
		assert.False(t, GetTaskArtifactStore().Enabled())
	})
}

func TestDecodeObjectStorageSetting(t *testing.T) {
	t.Run("empty raw means not configured", func(t *testing.T) {
		config, err := decodeObjectStorageSetting("")
		require.NoError(t, err)
		assert.Empty(t, config.Backend)
	})

	t.Run("invalid JSON rejected", func(t *testing.T) {
		_, err := decodeObjectStorageSetting("nope")
		require.Error(t, err)
	})
}

func TestBuildObjectStorageEnvImportPreview(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	require.NoError(t, model.DB.Where("key = ?", system_setting.ObjectStorageSettingOptionKey).Delete(&model.Option{}).Error)

	t.Run("preview never contains secret values", func(t *testing.T) {
		t.Setenv(system_setting.TaskArtifactStoreModeEnv, system_setting.TaskArtifactStoreModeS3)
		t.Setenv(system_setting.TaskArtifactStoreS3EndpointEnv, "https://s3.example.com")
		t.Setenv(system_setting.TaskArtifactStoreS3BucketEnv, "task-artifacts")
		t.Setenv(system_setting.TaskArtifactStoreS3RegionEnv, "us-east-1")
		t.Setenv(system_setting.TaskArtifactStoreS3AccessKeyEnv, "AKIAIOSFODNN7EXAMPLE")
		t.Setenv(system_setting.TaskArtifactStoreS3SecretKeyEnv, "SUPERSECRETVALUE")

		preview, ok := BuildObjectStorageEnvImportPreview()
		require.True(t, ok)
		encoded, err := common.Marshal(preview)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "SUPERSECRETVALUE")
		assert.NotContains(t, string(encoded), "AKIAIOSFODNN7EXAMPLE")
		assert.Equal(t, true, preview["credential_configured"])
	})
}

// fakeImageObjectStore 验证图片业务经能力接口分发，不依赖具体存储实现。
type fakeImageObjectStore struct {
	disabledArtifactStore
	putKey    string
	presigned string
	headed    bool
	fetched   []byte
}

func (f *fakeImageObjectStore) putImageObject(_ context.Context, objectKey, _ string, _ []byte) (*ImageObjectRef, error) {
	f.putKey = objectKey
	return &ImageObjectRef{ObjectKey: objectKey}, nil
}

func (f *fakeImageObjectStore) presignImageObjectURL(string) (string, int64, error) {
	return f.presigned, 300, nil
}

func (f *fakeImageObjectStore) headImageObject(_ context.Context, _ string) (bool, error) {
	return f.headed, nil
}

func (f *fakeImageObjectStore) fetchImageObjectBytes(context.Context, string) ([]byte, error) {
	return f.fetched, nil
}

func TestImageObjectFunctionsDispatchThroughCapabilityInterface(t *testing.T) {
	restoreRuntime(t)
	fake := &fakeImageObjectStore{presigned: "https://signed.example/1", headed: true, fetched: []byte("bytes")}
	require.True(t, taskArtifactStoreRuntime.swap(fake, "rev-fake"))

	ref, err := PutImageObject(t.Context(), "images/tasks/task_x/input-0", "image/png", []byte("x"))
	require.NoError(t, err)
	assert.Equal(t, "images/tasks/task_x/input-0", ref.ObjectKey)
	assert.Equal(t, "images/tasks/task_x/input-0", fake.putKey)

	url, expiry, err := PresignImageObjectURL(t.Context(), "k")
	require.NoError(t, err)
	assert.Equal(t, "https://signed.example/1", url)
	assert.Equal(t, int64(300), expiry)

	exists, err := HeadImageObject(t.Context(), "k")
	require.NoError(t, err)
	assert.True(t, exists)

	content, err := FetchImageObjectBytes(t.Context(), "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("bytes"), content)
}

func TestImageObjectFunctionsFailClosedWhenDisabled(t *testing.T) {
	restoreRuntime(t)
	applyTaskArtifactStoreSetting("")

	_, err := PutImageObject(t.Context(), "k", "image/png", []byte("x"))
	require.ErrorIs(t, err, ErrTaskArtifactStoreDisabled)
	_, _, err = PresignImageObjectURL(t.Context(), "k")
	require.ErrorIs(t, err, ErrTaskArtifactStoreDisabled)
	_, err = HeadImageObject(t.Context(), "k")
	require.ErrorIs(t, err, ErrTaskArtifactStoreDisabled)
	_, err = FetchImageObjectBytes(t.Context(), "k")
	require.ErrorIs(t, err, ErrTaskArtifactStoreDisabled)
}
