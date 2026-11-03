package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceImageAndVideoURLsStayOpaqueWithoutMediaDownloads(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		body string
	}{
		{"image", "/v1/images/generations", " {\"data\":[{\"url\":\"%s/image.png\"}],\"n\":1,\"n\":2}\n"},
		{"video", "/api/v3/contents/generations/tasks", " {\"content\":{\"video_url\":\"%s/video.mp4\"},\"duration\":4}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			var downloads atomic.Int64
			media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				downloads.Add(1)
				_, _ = w.Write([]byte("media must not enter evidence"))
			}))
			defer media.Close()
			body := []byte(fmt.Sprintf(test.body, media.URL))
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, body, got)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer provider.Close()
			engine := gin.New()
			engine.Use(APIRequestEvidence())
			engine.POST(test.path, func(c *gin.Context) {
				input, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, provider.URL, bytes.NewReader(input))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				response, err := (&http.Client{Transport: WithAPIRequestEvidenceTransport(nil)}).Do(req)
				require.NoError(t, err)
				defer response.Body.Close()
				c.Header("Content-Type", "application/json")
				_, err = io.Copy(c.Writer, response.Body)
				require.NoError(t, err)
			})
			request := httptest.NewRequest(http.MethodPost, test.path, bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, body, response.Body.Bytes())
			indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
			require.Len(t, indexes, 1)
			events := evidenceEventRows(t, indexes[0].Id)
			require.Len(t, events, 4)
			for _, event := range events {
				assert.Equal(t, body, evidenceCapturedBody(t, event), event.Stage)
			}
			assert.Zero(t, downloads.Load(), "capturing and reading a URL must not fetch its media")
		})
	}
}
