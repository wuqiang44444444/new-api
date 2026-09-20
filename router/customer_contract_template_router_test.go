package router

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractTemplateRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	require.NotPanics(t, func() {
		registerCustomerContractTemplateRoutes(engine.Group("/api"))
	})

	paths := map[string]bool{}
	for _, route := range engine.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	assert.True(t, paths[http.MethodGet+" /api/customer-contract-templates"])
	assert.True(t, paths[http.MethodGet+" /api/customer-contract-templates/options"])
	assert.True(t, paths[http.MethodPost+" /api/customer-contract-templates"])
	assert.True(t, paths[http.MethodGet+" /api/customer-contract-templates/:id"])
	assert.True(t, paths[http.MethodPut+" /api/customer-contract-templates/:id"])
	assert.True(t, paths[http.MethodGet+" /api/customer-contract-templates/:id/audits"])
}

func TestCustomerContractTemplateOptionsRouteBeatsWildcard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerCustomerContractTemplateRoutes(engine.Group("/api"))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/customer-contract-templates/options", nil))
	require.Equal(t, http.StatusUnauthorized, recorder.Code, "static /options must match its own route, not the :id wildcard")

	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/customer-contract-templates", nil))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, "unauthenticated template access is rejected")
}

func TestCustomerContractTemplateRoutesRejectAuthenticatedCustomer(t *testing.T) {
	setupRelayRouterTestDB(t)
	pat := "template-customer-fixture"
	user := model.User{Username: "template-customer", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, AccessToken: &pat, AuthVersion: 1, AffCode: "template-customer"}
	require.NoError(t, model.DB.Create(&user).Error)
	engine := gin.New()
	registerCustomerContractTemplateRoutes(engine.Group("/api"))
	registerBillingReconciliationRoutes(engine.Group("/api"))
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/customer-contract-templates"}, {"GET", "/api/customer-contract-templates/options"},
		{"GET", "/api/customer-contract-templates/1"}, {"GET", "/api/customer-contract-templates/1/audits"},
		{"POST", "/api/customer-contract-templates"}, {"PUT", "/api/customer-contract-templates/1"},
		{"POST", "/api/billing/admin/upstream-exports"},
	} {
		request := httptest.NewRequest(route.method, route.path, nil)
		request.Header.Set("Authorization", "Bearer "+pat)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code, route.method+" "+route.path)
	}
}
