package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceBusinessScopeIncludesTextAndMedia(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/embeddings", "/v1/rerank", "/v1/models", "/v1/batches", "/v1/files", "/v1beta/models/test:generateContent", "/pg/chat/completions", "/suno/submit/lyrics", "/v1/images/generations", "/api/v3/contents/generations/tasks"} {
		assert.NotEmpty(t, mediaEvidenceRequestKind(http.MethodPost, path), path)
	}
	for _, path := range []string{"/api/user/login", "/api/status", "/api/task_request_evidence", "/health", "/assets/app.js", "/api/channel/"} {
		assert.Empty(t, mediaEvidenceRequestKind(http.MethodPost, path), path)
	}
}
func TestEvidenceUploadExcludedBeforeAnyCopyAndResponseIndependent(t *testing.T) {
	for _, tc := range []struct{ path, contentType string }{
		{"/v1/files", "application/json"}, {"/fast/mj/submit/upload-discord-images", "application/json"}, {"/v1/images/edits", "multipart/form-data; boundary=test"}, {"/v1/audio/transcriptions", "application/octet-stream"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			setupEvidenceTestEnv(t)
			engine := gin.New()
			engine.Use(APIRequestEvidence())
			engine.POST(tc.path, func(c *gin.Context) {
				_, err := io.Copy(io.Discard, c.Request.Body)
				require.NoError(t, err)
				c.JSON(201, gin.H{"id": "file-reference"})
			})
			req := httptest.NewRequest("POST", tc.path, strings.NewReader("private file bytes"))
			req.Header.Set("Content-Type", tc.contentType)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			assert.Equal(t, 201, response.Code)
			items := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
			require.Len(t, items, 1)
			for _, event := range evidenceEventRows(t, items[0].Id) {
				_, status := ReadEvidenceEventContent(event, false)
				if event.Stage == model.TaskRequestEvidenceStageNorthReceive {
					assert.Equal(t, "missing", status)
				} else {
					assert.Equal(t, "available", status)
					assert.Contains(t, string(evidenceCapturedBody(t, event)), "file-reference")
				}
			}
		})
	}
}
