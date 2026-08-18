package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractAdminRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerCustomerContractAdminRoutes(engine.Group("/api"))

	routes := engine.Routes()
	found := false
	for _, route := range routes {
		if route.Method == http.MethodGet && route.Path == "/api/customer-contracts" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestCustomerContractAdminRouteRejectsUnauthenticatedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerCustomerContractAdminRoutes(engine.Group("/api"))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/customer-contracts", nil)

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
