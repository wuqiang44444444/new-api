package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func drainImageEvidence(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool { return len(imageEvidenceSlots) == 0 }, 3*time.Second, time.Millisecond)
}

// Only synthetic responses and temporary encrypted storage are used here.
func TestImageErrorEvidencePreservesRawResponsesInEncryptedStorage(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	t.Cleanup(func() { drainImageEvidence(t) })
	for _, tc := range []struct {
		name, body, contentType string
		status                  int
		transportErr            error
	}{
		{name: "full_error", body: `{"error":{"message":"` + strings.Repeat("upstream detail ", 4000) + `","api_key":"fake-hidden"},"max_tokens":4}`, contentType: "application/json", status: 400},
		{name: "sse_error", body: "data: {\"type\":\"error\",\"error\":{\"message\":\"raw stream failure\"}}\n\n", contentType: "text/event-stream", status: 200},
		{name: "transport", transportErr: errors.New("dial tcp: connection refused; fixture-provider-secret")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: tc.name, UserID: 7, AppID: 11})
			require.NotNil(t, capture)
			ctx := WithImageErrorEvidence(context.Background(), capture)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://provider.example/images?api_key=fake-index-secret", nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer fixture-provider-secret")
			var resp *http.Response
			if tc.status != 0 {
				resp = &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {tc.contentType}, "Set-Cookie": {"session=fake-secret"}}, Body: io.NopCloser(strings.NewReader(tc.body))}
			}
			ObserveImageHTTPExchange(ctx, req, resp, tc.transportErr, "generation")
			if resp != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(body))
				require.NoError(t, resp.Body.Close())
			}
			capture.Finish(false)
			drainImageEvidence(t)
			indices, count, queryErr := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: tc.name, Num: 10})
			require.NoError(t, queryErr)
			require.EqualValues(t, 1, count)
			events := evidenceEventRows(t, indices[0].Id)
			require.Len(t, events, 1)

			assert.Equal(t, tc.status, events[0].StatusCode)
			assert.NotContains(t, events[0].Target, "fake-index-secret")
			payload := evidenceCapturedBody(t, events[0])
			assert.Equal(t, tc.body, string(payload))
			content, status := ReadEvidenceEventContent(events[0], false)
			require.Equal(t, "available", status)
			if tc.transportErr != nil {
				assert.Equal(t, tc.transportErr.Error(), content.Meta.Error)
			}
			if tc.status != 0 {
				assert.Equal(t, []string{"session=fake-secret"}, content.Meta.Headers["Set-Cookie"])
			}
			disk, err := os.ReadFile(filepath.Join(env.storeDir, EvidenceObjectKey(events[0].EvidenceId, events[0].Id)))
			require.NoError(t, err)
			assert.NotContains(t, string(disk), "upstream detail")
			assert.NotContains(t, string(disk), "connection refused")
		})
	}
}

type blockedImageEvidenceStore struct {
	TaskRequestEvidenceObjectStore
	started chan struct{}
	release chan struct{}
}

func (s *blockedImageEvidenceStore) PutEventBody(evidenceID, eventID int64, payload []byte) error {
	return s.Put(EvidenceObjectKey(evidenceID, eventID), payload)
}

func (s *blockedImageEvidenceStore) Put(key string, payload []byte) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	<-s.release
	return errors.New("storage offline")
}

func TestImageErrorEvidenceCannotBlockBusinessAndReportsLoss(t *testing.T) {
	setupEvidenceTestEnv(t)
	original := evidenceObjectStore
	blocked := &blockedImageEvidenceStore{TaskRequestEvidenceObjectStore: original, started: make(chan struct{}, 1), release: make(chan struct{})}
	evidenceObjectStore = blocked
	t.Cleanup(func() { evidenceObjectStore = original })
	released := false
	t.Cleanup(func() {
		if !released {
			close(blocked.release)
		}
		drainImageEvidence(t)
	})
	beforeFailed, beforeDropped := imageEvidenceFailed.Load(), imageEvidenceDropped.Load()
	first := NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: "blocked"})
	require.NotNil(t, first)
	first.CaptureClientBody([]byte(`{"error":"original"}`))
	first.FinishClient(400, http.Header{"Content-Type": {"application/json"}})
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	for i := 1; i < cap(imageEvidenceSlots); i++ {
		c := NewImageErrorEvidence(model.TaskRequestEvidence{})
		require.NotNil(t, c)
		c.Finish(false)
	}
	assert.Nil(t, NewImageErrorEvidence(model.TaskRequestEvidence{}))
	assert.Equal(t, beforeDropped+1, imageEvidenceDropped.Load())
	close(blocked.release)
	released = true
	drainImageEvidence(t)
	assert.Equal(t, beforeFailed+1, imageEvidenceFailed.Load())
}

func TestImageErrorEvidenceTruncationDoesNotTruncateResponse(t *testing.T) {
	setupEvidenceTestEnv(t)
	t.Cleanup(func() { drainImageEvidence(t) })
	config := system_setting.GetTaskRequestEvidenceConfig()
	config.MaxResponseBytes = 16
	system_setting.SetTaskRequestEvidenceConfig(config)
	c := NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: "truncated"})
	require.NotNil(t, c)
	ctx := WithImageErrorEvidence(context.Background(), c)
	original := strings.Repeat("synthetic error text ", 100)
	resp := &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader(original))}
	ObserveImageHTTPExchange(ctx, nil, resp, nil, "generation")
	received, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, original, string(received))
	require.NoError(t, resp.Body.Close())
	c.Finish(true)
	drainImageEvidence(t)
	indices, _, queryErr := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "truncated", Num: 10})
	require.NoError(t, queryErr)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 1)

	assert.Equal(t, original[:16], string(evidenceCapturedBody(t, events[0])))
}

func TestImageClientEvidencePreservesSSEAndValidationResponse(t *testing.T) {
	setupEvidenceTestEnv(t)
	t.Cleanup(func() { drainImageEvidence(t) })
	for _, stream := range []bool{false, true} {
		name, body, status := "validation", `{"error":{"code":"invalid_request","message":"missing model"}}`, 400
		if stream {
			name, body, status = "stream", "data: {\"type\":\"error\",\"error\":{\"message\":\"provider failure\"}}\n\n", 200
		}
		t.Run(name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				capture := NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: name})
				require.NotNil(t, capture)
				defer CaptureImageClientResponse(c, capture)()
				c.Next()
			})
			engine.POST("/images", func(c *gin.Context) {
				contentType := "application/json"
				if stream {
					contentType = "text/event-stream"
				}
				c.Header("Content-Type", contentType)
				c.Status(status)
				_, err := c.Writer.WriteString(body)
				require.NoError(t, err)
				if stream {
					c.Writer.Flush()
				}
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/images", nil))
			assert.Equal(t, status, response.Code)
			assert.Equal(t, body, response.Body.String())
			assert.Equal(t, stream, response.Flushed)
			drainImageEvidence(t)
			indices, _, queryErr := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: name, Num: 10})
			require.NoError(t, queryErr)
			require.Len(t, indices, 1)
			events := evidenceEventRows(t, indices[0].Id)
			require.Len(t, events, 1)

			assert.Equal(t, status, events[0].StatusCode)
		})
	}
}

func TestImageErrorEvidencePreservesMalformedCapturedBytes(t *testing.T) {
	setupEvidenceTestEnv(t)
	t.Cleanup(func() { drainImageEvidence(t) })
	capture := NewImageErrorEvidence(model.TaskRequestEvidence{RequestID: "malformed"})
	require.NotNil(t, capture)
	capture.CaptureClientBody([]byte(`{"api_key":"synthetic-secret`))
	capture.FinishClient(502, http.Header{"Content-Type": {"application/json"}})
	drainImageEvidence(t)
	indices, _, queryErr := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "malformed", Num: 10})
	require.NoError(t, queryErr)
	require.Len(t, indices, 1)
	events := evidenceEventRows(t, indices[0].Id)
	require.Len(t, events, 1)

	payload := evidenceCapturedBody(t, events[0])
	assert.Equal(t, `{"api_key":"synthetic-secret`, string(payload))
}
