package controller

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidencePreviewMasksEncodedURLsWithoutChangingOriginal(t *testing.T) {
	evidence := setupEvidenceViewFixture(t)
	for i, tc := range []struct {
		name, contentType, body string
	}{
		{"uppercase", "text/plain", "HTTPS://example.test/video?sig=fixture-secret"},
		{"json escapes", "application/json", `{"url":"https:\/\/example.test/video?sig=fixture-secret","id":9007199254740993}`},
		{"form", "application/x-www-form-urlencoded", "url=https%3A%2F%2Fexample.test%2Fvideo%3Fsig%3Dfixture-secret&n=2"},
		{"multipart", "multipart/form-data; boundary=fixture", "--fixture\r\nContent-Disposition: form-data; name=\"request\"\r\nContent-Type: application/json\r\n\r\n{\"url\":\"HTTPS://example.test/video?sig=fixture-secret\"}\r\n--fixture\r\nContent-Disposition: form-data; name=\"options\"\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nurl=https%3A%2F%2Fexample.test%3Fsig%3Dfixture-secret\r\n--fixture--\r\n"},
		{"sse", "text/event-stream", "data: {\"url\":\"https:\\/\\/example.test/video?sig=fixture-secret\"}\n\ndata: [DONE]\n"},
		{"invalid URL", "text/plain", "https://example.test/%zz?sig=fixture-secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := fmt.Sprintf("preview/%d", i)
			require.NoError(t, service.GetTaskRequestEvidenceStore().Put(key, []byte(tc.body)))
			event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, ObjectKey: key, ContentType: tc.contentType, Sha256: service.EvidenceSha256Hex([]byte(tc.body)), Complete: true}
			require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
			params := []gin.Param{{Key: "id", Value: strconv.FormatInt(evidence.Id, 10)}, {Key: "event_id", Value: strconv.FormatInt(event.Id, 10)}}
			for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
				response := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, role, "/api/task_request_evidence", params...)
				require.Equal(t, 200, response.Code)
				assert.NotContains(t, response.Body.String(), "fixture-secret")
				var result struct {
					Data struct {
						Events []struct {
							ID      int64  `json:"id"`
							Status  string `json:"body_status"`
							Preview string `json:"preview"`
						} `json:"events"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				found := false
				for _, view := range result.Data.Events {
					if view.ID == event.Id {
						found = true
						assert.Equal(t, "available", view.Status)
						assert.NotEmpty(t, view.Preview)
					}
				}
				require.True(t, found)
				if tc.name == "json escapes" {
					assert.Contains(t, response.Body.String(), "9007199254740993")
				}
			}
			for _, handler := range []gin.HandlerFunc{GetTaskRequestEvidenceContent, GetTaskRequestEvidenceObject} {
				response := callEvidenceHandler(t, handler, common.RoleRootUser, "/api/task_request_evidence", params...)
				require.Equal(t, 200, response.Code)
				assert.Equal(t, tc.body, response.Body.String())
			}
		})
	}
}

func TestEvidenceMalformedPreviewNeverFallsBackToOriginal(t *testing.T) {
	for _, tc := range []struct{ contentType, body string }{
		{"application/json", `{"url":"https://example.test/?sig=fixture-secret"`},
		{"application/x-www-form-urlencoded", "url=%zz&sig=fixture-secret"},
		{"multipart/form-data; boundary=fixture", "--fixture\r\ninvalid-header\r\n\r\nfixture-secret\r\n--fixture--\r\n"},
	} {
		t.Run(tc.contentType, func(t *testing.T) {
			evidence := setupEvidenceViewFixture(t)
			require.NoError(t, service.GetTaskRequestEvidenceStore().Put("preview/malformed", []byte(tc.body)))
			event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, ObjectKey: "preview/malformed", ContentType: tc.contentType, Sha256: service.EvidenceSha256Hex([]byte(tc.body))}
			require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
			response := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, common.RoleAdminUser, "/api/task_request_evidence", gin.Param{Key: "id", Value: strconv.FormatInt(evidence.Id, 10)})
			assert.NotContains(t, response.Body.String(), "fixture-secret")
			assert.Contains(t, response.Body.String(), `"body_status":"read_failed"`)
		})
	}
}
