package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUserBillingStatementRejectsInvalidQueryBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		query       string
		wantMessage string
	}{
		{
			name:        "range longer than 31 days",
			query:       "start_timestamp=1000&end_timestamp=2765801",
			wantMessage: "time range cannot exceed 31 days",
		},
		{
			name:        "non-positive token id",
			query:       "start_timestamp=1000&end_timestamp=2000&token_id=-1",
			wantMessage: "invalid token_id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(
				http.MethodGet,
				"/api/billing/self?"+test.query,
				nil,
			)

			GetUserBillingStatement(context)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, test.wantMessage, response.Message)
		})
	}
}
