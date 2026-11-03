package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceRegisteredPluginRoutesDoNotDependOnVendorPrefix(t *testing.T) {
	setupEvidenceTestEnv(t)
	inner := gin.New()
	inner.Use(func(c *gin.Context) { c.Set("route_tag", "relay"); c.Next() }, APIRequestEvidence())
	for _, path := range []string{"/doubao/api/v3/images/generations", "/ali/api/v1/tasks/:task_id", "/custom/jobs"} {
		inner.POST(path, func(c *gin.Context) {
			c.Set("id", 7)
			c.Set("token_id", 3)
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			c.Data(201, "application/json", body)
		})
	}
	outer := gin.New()
	outer.Use(APIRequestEvidence())
	outer.NoRoute(func(c *gin.Context) { inner.ServeHTTP(c.Writer, c.Request); c.Abort() })
	const body = " {\"n\":0,\"n\":1}\n"
	for _, path := range []string{"/doubao/api/v3/images/generations", "/ali/api/v1/tasks/known", "/custom/jobs", "/not-registered"} {
		response := httptest.NewRecorder()
		outer.ServeHTTP(response, httptest.NewRequest("POST", path, strings.NewReader(body)))
		if path == "/not-registered" {
			assert.Equal(t, 404, response.Code)
		} else {
			assert.Equal(t, 201, response.Code)
			assert.Equal(t, body, response.Body.String())
		}
	}
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 3, "an unknown inner route must not create evidence")
	for _, index := range indexes {
		assert.Equal(t, 7, index.UserID)
		assert.Equal(t, 3, index.AppID)
		events := evidenceEventRows(t, index.Id)
		require.Len(t, events, 2)
		for _, event := range events {
			assert.Equal(t, body, string(evidenceCapturedBody(t, event)))
		}
	}
}

func TestEvidenceDeliveryNeverRestartsAfterInitialRecordBudgetFailure(t *testing.T) {
	setupEvidenceTestEnv(t)
	for i := 0; i < cap(evidenceRecordSlots); i++ {
		evidenceRecordSlots <- struct{}{}
	}
	released := false
	defer func() {
		if !released {
			for i := 0; i < cap(evidenceRecordSlots); i++ {
				<-evidenceRecordSlots
			}
		}
	}()
	engine := gin.New()
	engine.Use(APIRequestEvidence())
	engine.GET("/v1/responses", func(c *gin.Context) {
		_, err := c.Writer.WriteString("first chunk\n")
		require.NoError(t, err)
		c.Writer.Flush()
		for i := 0; i < cap(evidenceRecordSlots); i++ {
			<-evidenceRecordSlots
		}
		released = true
		_, err = c.Writer.WriteString("last chunk\n")
		require.NoError(t, err)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/responses", nil))
	assert.Equal(t, "first chunk\nlast chunk\n", response.Body.String())
	assert.True(t, response.Flushed)
	assert.Empty(t, evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{}), "a suffix cannot become a new captured object")
}

func TestEvidenceKnownFileDeliveryBypassesBodyRegardlessOfContentType(t *testing.T) {
	for _, contentType := range []string{"application/json", "text/plain", ""} {
		t.Run(contentType, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			engine := gin.New()
			engine.Use(APIRequestEvidence())
			engine.GET("/v1/tasks/:key/artifacts/:artifact_key/content", func(c *gin.Context) {
				c.Header("Content-Type", contentType)
				_, err := c.Writer.Write([]byte("known file bytes"))
				require.NoError(t, err)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest("GET", "/v1/tasks/known/artifacts/file/content", nil))
			assert.Equal(t, "known file bytes", response.Body.String())
			indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
			require.Len(t, indexes, 1)
			events := evidenceEventRows(t, indexes[0].Id)
			require.Len(t, events, 2)
			for _, event := range events {
				_, status := ReadEvidenceEventContent(event, false)
				assert.Equal(t, "missing", status)
			}
		})
	}
}

func TestEvidenceS3OperationsCannotReenterObservationThroughTransportOrFallback(t *testing.T) {
	setupEvidenceTestEnv(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, err := io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"storage_response\":true}"))
	}))
	defer server.Close()
	artifact, err := NewS3ArtifactStore(system_setting.TaskArtifactStoreConfig{
		Mode: system_setting.TaskArtifactStoreModeS3, S3Endpoint: server.URL,
		S3Region: "us-east-1", S3Bucket: "fixture", S3PresignTTLSeconds: 300, S3AccessKey: "fixture", S3SecretKey: "fixture",
	})
	require.NoError(t, err)
	store := artifact.(*s3ArtifactStore)
	store.httpClient.Transport = WithAPIRequestEvidenceTransport(nil)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{})
	ctx := WithImageErrorEvidence(context.Background(), capture)
	n, err := store.putObject(ctx, "file.json", "application/json", bytes.NewReader([]byte("{\"file\":true}")))
	require.NoError(t, err)
	assert.Positive(t, n)
	exists, err := store.HeadObject(ctx, "file.json")
	require.NoError(t, err)
	assert.True(t, exists)
	capture.Finish(false)
	assert.Equal(t, 2, calls)
	assert.Empty(t, evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{}))
}

func TestEvidenceIdentityIsObservedOncePerBodyInsteadOfPerRead(t *testing.T) {
	setupEvidenceTestEnv(t)
	calls := 0
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{})
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("body"))
	record := capture.requestRecord(req, model.TaskRequestEvidenceStageNorthReceive)
	body := &imageEvidenceBody{ReadCloser: req.Body, capture: capture, record: record, observeIdentity: func() { calls++; capture.mu.Lock(); capture.Index.UserID = 7; capture.mu.Unlock() }}
	_, err := io.CopyBuffer(io.Discard, body, make([]byte, 1))
	require.NoError(t, err)
	require.NoError(t, body.Close())
	capture.Finish(false)
	assert.Equal(t, 1, calls)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	assert.Equal(t, 7, indexes[0].UserID)
}

func TestEvidenceHeaderSnapshotRemainsIndependentOfLiveRequest(t *testing.T) {
	setupEvidenceTestEnv(t)
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{})
	request := httptest.NewRequest("POST", "/v1/responses", nil)
	request.Header.Set("X-Fixture", "original")
	record := capture.requestRecord(request, model.TaskRequestEvidenceStageNorthReceive)
	request.Header.Set("X-Fixture", "changed later")
	record.append([]byte("opaque body"))
	record.close()
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
	require.Len(t, indexes, 1)
	events := evidenceEventRows(t, indexes[0].Id)
	require.Len(t, events, 1)
	content, status := ReadEvidenceEventContent(events[0], false)
	require.Equal(t, "available", status)
	assert.Equal(t, []string{"original"}, content.Meta.Headers["X-Fixture"])
}
