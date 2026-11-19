package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageLogConsoleRoutesAreIsolatedFromLegacyList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := gin.New()
	legacyReads := 0
	old.GET("/api/log/", func(c *gin.Context) {
		legacyReads++
		c.Status(http.StatusOK)
	})
	old.NoRoute(controller.RelayNotFound)
	current := gin.New()
	registerUsageLogConsoleRoutes(current.Group("/api/log"))

	for path, scope := range map[string]string{
		"/api/log/console/page":      "log:read",
		"/api/log/console/stat":      "log:read",
		"/api/log/self/console/page": "usage:read",
		"/api/log/self/console/stat": "usage:read",
	} {
		t.Run(path, func(t *testing.T) {
			rule, ok := middleware.AccessTokenRouteRule(http.MethodGet + " " + path)
			require.True(t, ok)
			assert.Equal(t, scope, rule.Scope())

			olderResponse := httptest.NewRecorder()
			old.ServeHTTP(olderResponse, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusNotFound, olderResponse.Code)
			assert.Zero(t, legacyReads)

			currentResponse := httptest.NewRecorder()
			current.ServeHTTP(currentResponse, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusUnauthorized, currentResponse.Code)
		})
	}
}
