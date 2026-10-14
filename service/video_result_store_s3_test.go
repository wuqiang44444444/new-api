package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeS3VideoStore 是 S3 兼容对象存储的内存假实现：覆盖发起分片、上传分片、
// 完成、中止与整体 PUT 五类请求，记录对象与分片事实以验证分片上传行为。
type fakeS3VideoStore struct {
	mu             sync.Mutex
	objects        map[string][]byte
	uploadID       string
	parts          map[int][]byte
	aborted        bool
	completed      int
	failStage      string
	mimeType       string
	completionBody *string
}

func newFakeS3VideoStore() *fakeS3VideoStore {
	return &fakeS3VideoStore{objects: map[string][]byte{}, parts: map[int][]byte{}}
}

func (f *fakeS3VideoStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && r.URL.Query().Has("uploads"):
		f.uploadID = "upload-fixture-1"
		f.mimeType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", f.uploadID)
	case r.Method == http.MethodPut && r.URL.Query().Has("uploadId"):
		if f.failStage == "part" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		partNumber := 0
		_, _ = fmt.Sscanf(r.URL.Query().Get("partNumber"), "%d", &partNumber)
		data, _ := io.ReadAll(r.Body)
		f.parts[partNumber] = data
		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, partNumber))
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && r.URL.Query().Has("uploadId"):
		if f.completionBody != nil {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, *f.completionBody)
			return
		}
		if f.failStage == "complete" {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "<Error><Code>MalformedXML</Code></Error>")
			return
		}
		f.completed++
		joined := make([]byte, 0)
		for i := 1; ; i++ {
			part, ok := f.parts[i]
			if !ok {
				break
			}
			joined = append(joined, part...)
		}
		f.objects[r.URL.Path] = joined
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><ETag>"complete-etag"</ETag></CompleteMultipartUploadResult>`)
	case r.Method == http.MethodDelete && r.URL.Query().Has("uploadId"):
		f.aborted = true
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPut:
		data, _ := io.ReadAll(r.Body)
		f.objects[r.URL.Path] = data
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newS3VideoResultTestStore(t *testing.T, fake http.Handler) *s3ArtifactStore {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	config := system_setting.TaskArtifactStoreConfig{
		Mode:                system_setting.TaskArtifactStoreModeS3,
		S3Endpoint:          server.URL,
		S3Bucket:            "video-bucket",
		S3Region:            "us-east-1",
		S3AccessKey:         "test-key",
		S3SecretKey:         "test-secret",
		S3PresignTTLSeconds: system_setting.DefaultTaskArtifactStorePresignTTLSeconds,
	}
	store, err := NewS3ArtifactStore(config)
	require.NoError(t, err)
	return store.(*s3ArtifactStore)
}

func TestS3VideoResultPutSmallObjectUsesSinglePut(t *testing.T) {
	fake := newFakeS3VideoStore()
	store := newS3VideoResultTestStore(t, fake)
	ctx := context.Background()

	size, err := store.putVideoResultObject(ctx, "videos/tasks/task_s3_small/video", "video/mp4", strings.NewReader("small-video"), videoResultMaxBytes)
	require.NoError(t, err)
	assert.Equal(t, int64(len("small-video")), size)
	assert.Equal(t, "small-video", string(fake.objects["/video-bucket/videos/tasks/task_s3_small/video"]))
	assert.Empty(t, fake.uploadID, "small objects must not open a multipart upload")
}

func TestS3VideoResultPutLargeObjectUsesMultipart(t *testing.T) {
	fake := newFakeS3VideoStore()
	store := newS3VideoResultTestStore(t, fake)
	ctx := context.Background()

	// 16MB 分片 + 尾片：验证分片读取、ETag 顺序与完成重组。
	payload := make([]byte, videoResultUploadPartSize+4096)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	size, err := store.putVideoResultObject(ctx, "videos/tasks/task_s3_large/video", "video/mp4", strings.NewReader(string(payload)), videoResultMaxBytes)
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), size)
	assert.Equal(t, 2, len(fake.parts))
	assert.Equal(t, payload, fake.objects["/video-bucket/videos/tasks/task_s3_large/video"])
	assert.Equal(t, 1, fake.completed)
	assert.Equal(t, "video/mp4", fake.mimeType)
	assert.False(t, fake.aborted)
}

func TestS3VideoResultPartFailureAbortsUpload(t *testing.T) {
	fake := newFakeS3VideoStore()
	fake.failStage = "part"
	store := newS3VideoResultTestStore(t, fake)
	ctx := context.Background()

	payload := make([]byte, videoResultUploadPartSize+16)
	_, err := store.putVideoResultObject(ctx, "videos/tasks/task_s3_abort/video", "video/mp4", strings.NewReader(string(payload)), videoResultMaxBytes)
	require.Error(t, err)
	assert.True(t, fake.aborted, "failed parts must abort the multipart upload")
	assert.NotContains(t, fake.objects, "/video-bucket/videos/tasks/task_s3_abort/video", "partial uploads must never become objects")
}

func TestS3VideoResultCompletionFailureAborts(t *testing.T) {
	fake := newFakeS3VideoStore()
	fake.failStage = "complete"
	store := newS3VideoResultTestStore(t, fake)
	ctx := context.Background()

	payload := make([]byte, videoResultUploadPartSize+16)
	_, err := store.putVideoResultObject(ctx, "videos/tasks/task_s3_complete/video", "video/mp4", strings.NewReader(string(payload)), videoResultMaxBytes)
	require.Error(t, err)
	assert.True(t, fake.aborted)
	assert.Empty(t, fake.objects)
}

func TestS3VideoResultPresignCoversFullDay(t *testing.T) {
	store := newS3VideoResultTestStore(t, newFakeS3VideoStore())
	url, expiresAt, err := store.presignVideoResultObjectURL("videos/tasks/task_s3_sign/video", http.MethodGet, time.Duration(model.VideoResultPresignTTLSeconds)*time.Second)
	require.NoError(t, err)
	assert.Contains(t, url, "X-Amz-Expires=86400")
	assert.Contains(t, url, "X-Amz-Signature=")
	assert.InDelta(t, time.Now().Add(24*time.Hour).Unix(), expiresAt, 90)
}

func TestS3VideoResultReadsBodyAfterResponseHeaders(t *testing.T) {
	release := make(chan struct{})
	store := newS3VideoResultTestStore(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-release
		_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>delayed-id</UploadId></InitiateMultipartUploadResult>`)
	}))
	resp, err := store.videoResultRequest(context.Background(), http.MethodPost, store.objectURL("videos/tasks/delayed/video"), "uploads", "video/mp4", nil)
	close(release)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "receiving headers must not cancel the response body")
	assert.Contains(t, string(body), "<UploadId>delayed-id</UploadId>")
}

func TestS3VideoResultRequiresCompletionEvidence(t *testing.T) {
	for _, body := range []string{"", "<CompleteMultipartUploadResult>", "<Error><Code>InternalError</Code></Error>", "<CompleteMultipartUploadResult/>", "<Other><ETag>etag</ETag></Other>", "<CompleteMultipartUploadResult><ETag>etag</ETag></CompleteMultipartUploadResult><Error/>"} {
		t.Run(fmt.Sprintf("body_%d", len(body)), func(t *testing.T) {
			fake := newFakeS3VideoStore()
			fake.completionBody = &body
			store := newS3VideoResultTestStore(t, fake)
			_, err := store.putVideoResultObject(context.Background(), "videos/tasks/unconfirmed/video", "video/mp4", strings.NewReader(strings.Repeat("v", int(videoResultUploadPartSize)+1)), videoResultMaxBytes)
			require.Error(t, err)
			assert.True(t, fake.aborted, "unconfirmed completion must not be published")
		})
	}
}

func TestS3VideoResultInclusiveSizeLimit(t *testing.T) {
	for _, size := range []int{7, 8, 9} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			fake := newFakeS3VideoStore()
			store := newS3VideoResultTestStore(t, fake)
			uploaded, err := store.putVideoResultObject(context.Background(), "videos/tasks/bounded/video", "video/mp4", strings.NewReader(strings.Repeat("v", size)), 8)
			if size > 8 {
				require.ErrorIs(t, err, errVideoResultTooLarge)
				assert.Empty(t, fake.objects)
			} else {
				require.NoError(t, err)
				assert.EqualValues(t, size, uploaded)
			}
		})
	}
}
