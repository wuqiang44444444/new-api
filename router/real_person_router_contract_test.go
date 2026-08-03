package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRealPersonRouterUsesUnifiedRuleAndVerificationContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetAssetRouter(engine)
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, expected := range []string{
		http.MethodGet + " /v1/api-service-rules/current",
		http.MethodPost + " /v1/api-service-rules/acceptance",
		http.MethodPost + " /v1/real-person-authorizations",
		http.MethodGet + " /verification/real-person/:token",
		http.MethodHead + " /verification/real-person/:token",
		http.MethodGet + " /verification/real-person/complete",
	} {
		_, ok := routes[expected]
		assert.True(t, ok, expected)
	}

	for _, removed := range []string{
		http.MethodPost + " /v1/real-person-authorizations/:authorization_id/asset",
		http.MethodGet + " /consent/real-person/:token",
		http.MethodGet + " /consent/real-person/verify/:token",
		http.MethodGet + " /consent/real-person/receipt/:receipt_token",
		http.MethodPost + " /api/real-person-consents/:token/accept",
		http.MethodPost + " /api/real-person-consents/:token/reject",
		http.MethodGet + " /api/asset-consent-policies",
		http.MethodPost + " /api/asset-consent-policies",
	} {
		_, ok := routes[removed]
		assert.False(t, ok, removed)
	}
}
