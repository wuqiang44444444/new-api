package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionTestImageStore struct {
	disabledArtifactStore
	objects  map[string][]byte
	puts     int
	reads    int
	putErr   error
	afterPut func()
}

func (s *sessionTestImageStore) putImageObject(ctx context.Context, key, mime string, data []byte) (*ImageObjectRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.puts++
	if s.putErr != nil {
		return nil, s.putErr
	}
	if s.objects == nil {
		s.objects = make(map[string][]byte)
	}
	s.objects[key] = append([]byte(nil), data...)
	if s.afterPut != nil {
		s.afterPut()
	}
	return &ImageObjectRef{ObjectKey: key, MimeType: mime, Size: int64(len(data))}, nil
}

func (s *sessionTestImageStore) fetchImageObjectBytes(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.reads++
	data, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing object")
	}
	return data, nil
}

func (s *sessionTestImageStore) headImageObject(ctx context.Context, key string) (bool, error) {
	_, ok := s.objects[key]
	return ok, ctx.Err()
}

func (s *sessionTestImageStore) presignImageObjectURL(key string) (string, int64, error) {
	if _, ok := s.objects[key]; !ok {
		return "", 0, errors.New("wrong store")
	}
	return "https://storage.example/" + key, time.Now().Add(300 * time.Second).Unix(), nil
}

func TestImageOperationPinsStorageDuringRotation(t *testing.T) {
	restoreRuntime(t)
	oldStore, newStore := &sessionTestImageStore{}, &sessionTestImageStore{}
	taskArtifactStoreRuntime.swap(oldStore, "old")
	oldStore.afterPut = func() { taskArtifactStoreRuntime.swap(newStore, "new") }
	url, apiErr := PutEphemeralImageResult(t.Context(), "image/png", []byte("image"))
	require.Nil(t, apiErr, "PUT and signing must use the same client")
	assert.Contains(t, url, "images/ephemeral/")
	assert.Equal(t, 1, oldStore.puts)
	assert.Zero(t, newStore.puts)

	taskArtifactStoreRuntime.swap(oldStore, "old-again")
	ctx, err := WithImageObjectStore(t.Context())
	require.NoError(t, err)
	_, err = PutImageObject(ctx, "images/tasks/t/result-0", "image/png", []byte("first"))
	require.NoError(t, err)
	nested, err := WithImageObjectStore(ctx)
	require.NoError(t, err)
	_, err = PutImageObject(nested, "images/tasks/t/result-1", "image/png", []byte("second"))
	require.NoError(t, err)
	content, err := FetchImageObjectBytes(nested, "images/tasks/t/result-0")
	require.NoError(t, err)
	assert.Equal(t, []byte("first"), content)
	exists, err := HeadImageObject(nested, "images/tasks/t/result-1")
	require.NoError(t, err)
	assert.True(t, exists)
	_, _, err = PresignImageObjectURL(nested, "images/tasks/t/result-1")
	require.NoError(t, err)
	assert.Empty(t, newStore.objects)
}

func TestImageReadinessCachesPerClientAndFailsClosed(t *testing.T) {
	restoreRuntime(t)
	store := &sessionTestImageStore{}
	taskArtifactStoreRuntime.swap(store, "ready")
	require.NoError(t, CheckImageObjectStoreReady(t.Context()))
	require.NoError(t, CheckImageObjectStoreReady(t.Context()))
	assert.Equal(t, 1, store.puts, "a fresh observation avoids another network probe")
	assert.Equal(t, 1, store.reads)
	for key := range store.objects {
		assert.Contains(t, key, "object-storage-health/")
	}
	store.putErr = errors.New("write permission revoked")
	_, err := PutImageObject(t.Context(), "images/tasks/t/result-0", "image/png", []byte("image"))
	require.Error(t, err)
	require.Error(t, CheckImageObjectStoreReady(t.Context()), "a real write failure invalidates cached readiness")

	broken := &sessionTestImageStore{putErr: errors.New("credentials revoked")}
	taskArtifactStoreRuntime.swap(broken, "revoked")
	require.Error(t, CheckImageObjectStoreReady(t.Context()))
	require.Error(t, CheckImageObjectStoreReady(t.Context()))
	assert.Equal(t, 1, broken.puts, "failed probes are also briefly cached")
	assert.Zero(t, broken.reads)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, CheckImageObjectStoreReady(ctx), context.Canceled)
	taskArtifactStoreRuntime.swap(&disabledArtifactStore{}, "disabled")
	require.ErrorIs(t, CheckImageObjectStoreReady(t.Context()), ErrTaskArtifactStoreDisabled)
}

// Constant-space reader tests the actual byte boundary without random or stress inputs.
type imageLimitReader struct{ remaining int64 }

func (r *imageLimitReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > r.remaining {
		n = r.remaining
	}
	clear(p[:n])
	r.remaining -= n
	return int(n), nil
}

func TestImageObjectReadRejectsTruncation(t *testing.T) {
	for _, extra := range []int64{0, 1} {
		data, err := readImageObjectBytes(&imageLimitReader{remaining: fetchImageObjectMaxBytes + extra})
		if extra == 0 {
			require.NoError(t, err)
			assert.Len(t, data, fetchImageObjectMaxBytes)
		} else {
			require.ErrorContains(t, err, "size limit")
			assert.Nil(t, data, "never expose a partial image")
		}
	}
}

func TestImageObjectReadCancellationReachesBothBackends(t *testing.T) {
	for _, backend := range []string{"s3", "azure_blob"} {
		t.Run(backend, func(t *testing.T) {
			restoreRuntime(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("canceled reads must not start network requests")
			}))
			t.Cleanup(server.Close)
			config := system_setting.ObjectStorageConfig{Backend: backend, Endpoint: server.URL, Bucket: "images", AccountName: "account", Region: "us-east-1"}
			var store TaskArtifactStore
			var err error
			if backend == "s3" {
				store, err = NewS3ArtifactStore(legacyS3Config(config, "secret"))
			} else {
				store, err = NewAzureBlobArtifactStore(config, "dGVzdC1rZXk=")
			}
			require.NoError(t, err)
			taskArtifactStoreRuntime.swap(store, backend)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			data, err := FetchImageObjectBytes(ctx, "images/tasks/t/input-0")
			require.Error(t, err)
			assert.Nil(t, data)
		})
	}
}

func TestImageWorkerStorageOutageDoesNotSendOrRefund(t *testing.T) {
	truncate(t)
	restoreRuntime(t)
	seedImageTaskUser(t, 9918, 1000)
	task := newWorkerImageTask(9918, 100)
	require.NoError(t, model.InsertImageTask(model.ImageTaskInsertParams{Task: task,
		GlobalScope: model.ImageTaskAdmissionScopeGlobal(), AppScope: model.ImageTaskAdmissionScopeApp(task.UserId, task.AppID)}))
	previousExecutor := ImageTaskExecuteFunc
	t.Cleanup(func() { ImageTaskExecuteFunc = previousExecutor })
	calls := 0
	ImageTaskExecuteFunc = func(ctx context.Context, task *model.Task) ImageTaskExecution {
		calls++
		assert.Positive(t, reloadTask(t, task.ID).PrivateData.ImageTask.SentAt)
		return ImageTaskExecution{Outcome: ImageTaskOutcomeUnknown, FailureCode: "send_outcome_unknown"}
	}
	config := system_setting.LoadImageTaskConfig()
	taskArtifactStoreRuntime.swap(&sessionTestImageStore{putErr: errors.New("offline")}, "offline")
	executeQueuedImageTasks(t.Context(), config)
	stored := reloadTask(t, task.ID)
	assert.EqualValues(t, model.TaskStatusQueued, stored.Status)
	assert.Zero(t, stored.PrivateData.ImageTask.SentAt)
	assert.Zero(t, stored.PrivateData.ImageTask.LeaseAt)
	assert.Equal(t, 900, getUserQuota(t, 9918))
	assert.Zero(t, calls)
	taskArtifactStoreRuntime.swap(&sessionTestImageStore{}, "recovered")
	executeQueuedImageTasks(t.Context(), config)
	executeQueuedImageTasks(t.Context(), config)
	assert.Equal(t, 1, calls, "recovery sends once, unknown is never requeued")
	assert.Equal(t, model.TaskStatusReconciliationRequired, reloadTask(t, task.ID).Status)
	assert.Equal(t, 900, getUserQuota(t, 9918))
}
