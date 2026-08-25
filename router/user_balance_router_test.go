package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserBalanceRouteUsesPublicAPIPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerUserBalanceRoutes(engine.Group("/api"))

	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	_, exists := routes[http.MethodGet+" /api/user/balance"]
	assert.True(t, exists)
}

func TestUserBalanceRouteRequiresAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerUserBalanceRoutes(engine.Group("/api"))

	request := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
