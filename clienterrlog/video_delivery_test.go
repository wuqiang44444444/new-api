package clienterrlog_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRecorderSkipsOnlyMarkedVideoContentWait(t *testing.T) {
	for _, tc := range []struct {
		name, route, request string
		status               int
		mark, stream         bool
		wantRecords          uint64
	}{
		{"preparing", "/v1/videos/:task_id/content", "/v1/videos/task/content", 409, true, false, 0},
		{"unmarked conflict", "/v1/videos/:task_id/content", "/v1/videos/task/content", 409, false, false, 1},
		{"different route", "/v1/other", "/v1/other", 409, true, false, 1},
		{"throttled", "/v1/videos/:task_id/content", "/v1/videos/task/content", 429, true, false, 1},
		{"service failure", "/v1/videos/:task_id/content", "/v1/videos/task/content", 503, true, false, 1},
		{"explicit stream failure", "/v1/videos/:task_id/content", "/v1/videos/task/content", 409, true, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.GET(tc.route, func(c *gin.Context) {
				c.Set("route_tag", "relay")
				clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
				if tc.mark {
					clienterrlog.MarkVideoDeliveryWaiting(c.Request.Context())
				}
				if tc.stream {
					clienterrlog.AttachStreamError(c.Request.Context(), clienterrlog.StreamErrorReport{Reason: "upstream_read_error"})
				}
				c.Status(tc.status)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.request, nil))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.wantRecords, capture.Health().Accepted)
		})
	}
}
