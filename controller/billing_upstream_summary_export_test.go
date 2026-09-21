package controller

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSummaryExportRejectsUnscopedAndMixedScopes(t *testing.T) {
	setupBillingURLNameControllerTest(t)
	router := newBillingAdminTestRouter()
	router.POST("/exports", CreateAdminUpstreamExport)
	for _, body := range []string{
		`{"job_type":"upstream_summary"}`,
		`{"job_type":"upstream_summary","url_key":" "}`,
		`{"job_type":"upstream_summary","url_key":"https://provider.example","channel_id":1}`,
		`{"job_type":"upstream_summary","url_key":"https://provider.example","channel_id":0}`,
	} {
		t.Run(body, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/exports", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			var result struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
			assert.Equal(t, "upstream summary requires one URL group", result.Message)
		})
	}
}
