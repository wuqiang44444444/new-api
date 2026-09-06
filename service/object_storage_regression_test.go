package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectStoragePrefixDelivery(t *testing.T) {
	for _, backend := range []string{"s3", "azure_blob"} {
		t.Run(backend, func(t *testing.T) {
			var mu sync.Mutex
			objects := map[string][]byte{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method {
				case http.MethodPut:
					data, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					objects[r.URL.Path] = data
					w.Header().Set("ETag", `"test"`)
					w.WriteHeader(http.StatusCreated)
				case http.MethodGet, http.MethodHead:
					data, ok := objects[r.URL.Path]
					if !ok {
						w.Header().Set("x-ms-error-code", "BlobNotFound")
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					w.WriteHeader(http.StatusOK)
					if r.Method == http.MethodGet {
						_, _ = w.Write(data)
					}
				}
			}))
			t.Cleanup(server.Close)
			config := system_setting.ObjectStorageConfig{Backend: backend, Endpoint: server.URL, Bucket: "artifacts", Prefix: "production/", Region: "us-east-1", AccountName: "account"}
			var store TaskArtifactStore
			var err error
			if backend == "azure_blob" {
				store, err = NewAzureBlobArtifactStore(config, "dGVzdC1rZXk=")
			} else {
				store, err = NewS3ArtifactStore(legacyS3Config(config, "secret"))
			}
			require.NoError(t, err)
			imageStore := store.(imageObjectStore)
			key := "images/tasks/task_test/result-0"
			payload := []byte("image data")
			ref, err := imageStore.putImageObject(t.Context(), key, "image/png", payload)
			require.NoError(t, err)
			assert.Equal(t, key, ref.ObjectKey)
			mu.Lock()
			assert.Equal(t, payload, objects["/artifacts/production/"+key])
			mu.Unlock()
			exists, err := imageStore.headImageObject(t.Context(), ref.ObjectKey)
			require.NoError(t, err)
			assert.True(t, exists)
			data, err := imageStore.fetchImageObjectBytes(t.Context(), ref.ObjectKey)
			require.NoError(t, err)
			assert.Equal(t, payload, data)
			signedURL, _, err := imageStore.presignImageObjectURL(ref.ObjectKey)
			require.NoError(t, err)
			parsed, err := url.Parse(signedURL)
			require.NoError(t, err)
			assert.Equal(t, "/artifacts/production/"+key, parsed.Path)
			task := &model.Task{TaskID: "task_test", ClientProtocol: model.TaskClientProtocolImageOpenAIV1}
			task.PrivateData.ImageTask = &model.TaskImageExecutionData{Artifacts: []model.TaskImageArtifact{{ObjectKey: key, MimeType: "image/png"}}}
			resolved, err := store.Resolve(task, "result-0")
			require.NoError(t, err)
			require.NotNil(t, resolved)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/artifact", nil)
			require.NoError(t, store.Serve(c, task, resolved))
			parsed, err = url.Parse(w.Header().Get("Location"))
			require.NoError(t, err)
			assert.Equal(t, "/artifacts/production/"+key, parsed.Path)
			persisted, err := store.Persist(t.Context(), task, hosttypes.TaskArtifact{Key: "result-0", MimeType: "image/png"}, bytes.NewReader(payload))
			require.NoError(t, err)
			assert.Equal(t, key, persisted.ObjectKey)
			mu.Lock()
			assert.Len(t, objects, 1, "all delivery paths use the same object")
			mu.Unlock()
		})
	}
}

func TestObjectStorageSignedDownloadFailureIsRedacted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/redirect-target?sig=secret", http.StatusFound)
	}))
	_, err := fetchImageObjectViaPresignedURL(context.Background(), server.URL+"?sig=secret")
	require.Error(t, err)
	assert.EqualError(t, err, "object storage download failed")
	assert.NotContains(t, err.Error(), "secret")
	server.Close()
	_, err = fetchImageObjectViaPresignedURL(context.Background(), server.URL+"?sig=secret")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sig=")
	assert.NotContains(t, err.Error(), server.URL)
}

func TestObjectStorageProbeCleanupDeterminesSuccess(t *testing.T) {
	for _, failure := range []string{"none", "delete", "still_exists"} {
		t.Run(failure, func(t *testing.T) {
			var mu sync.Mutex
			var data []byte
			exists := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch r.Method {
				case http.MethodPut:
					data, _ = io.ReadAll(r.Body)
					exists = true
					w.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					_, _ = w.Write(data)
				case http.MethodHead:
					if !exists {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				case http.MethodDelete:
					if failure == "delete" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					exists = failure == "still_exists"
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			t.Cleanup(server.Close)
			config := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: server.URL, Bucket: "artifacts", Region: "us-east-1", AccountName: "access"}
			result := RunObjectStorageConnectionTest(config, "secret")
			assert.Equal(t, failure == "none", result.Success)
			assert.Equal(t, failure != "none", result.CleanupFailed)
			if failure != "none" {
				assert.Equal(t, "test object cleanup failed", result.Message)
			}
		})
	}
}
