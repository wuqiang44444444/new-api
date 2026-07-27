package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenModelAccessEnforcesAllowListAndPreservesBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	tests := []struct {
		name       string
		model      string
		limits     map[string]bool
		wantStatus int
		wantNext   bool
	}{
		{name: "allowed", model: "allowed-model", limits: map[string]bool{"allowed-model": true}, wantStatus: http.StatusNoContent, wantNext: true},
		{name: "forbidden", model: "blocked-model", limits: map[string]bool{"allowed-model": true}, wantStatus: http.StatusForbidden},
		{name: "empty limit", model: "blocked-model", wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			nextCalled := false
			engine.Use(BodyStorageCleanup())
			engine.POST("/v1/assets", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
				if test.limits != nil {
					common.SetContextKey(c, constant.ContextKeyTokenModelLimit, test.limits)
				}
				c.Next()
			}, TokenModelAccess(), func(c *gin.Context) {
				var body struct {
					Model string `json:"model"`
				}
				require.NoError(t, c.ShouldBindJSON(&body))
				assert.Equal(t, test.model, body.Model)
				nextCalled = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/assets", strings.NewReader(`{"model":"`+test.model+`"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			engine.ServeHTTP(recorder, request)

			assert.Equal(t, test.wantStatus, recorder.Code)
			assert.Equal(t, test.wantNext, nextCalled)
		})
	}
}
