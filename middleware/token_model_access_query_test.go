package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenModelAccessFromQueryEnforcesAllowList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())

	for _, test := range []struct {
		name       string
		model      string
		limits     map[string]bool
		wantStatus int
		wantNext   bool
	}{
		{name: "allowed", model: "allowed-model", limits: map[string]bool{"allowed-model": true}, wantStatus: http.StatusNoContent, wantNext: true},
		{name: "forbidden", model: "blocked-model", limits: map[string]bool{"allowed-model": true}, wantStatus: http.StatusForbidden},
		{name: "missing limits", model: "blocked-model", wantStatus: http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			nextCalled := false
			engine.GET("/v1/assets/:asset_id", func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
				if test.limits != nil {
					common.SetContextKey(c, constant.ContextKeyTokenModelLimit, test.limits)
				}
				c.Next()
			}, TokenModelAccessFromQuery(), func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, "/v1/assets/asset-1?model="+test.model, nil)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)

			assert.Equal(t, test.wantStatus, recorder.Code)
			assert.Equal(t, test.wantNext, nextCalled)
		})
	}
}
