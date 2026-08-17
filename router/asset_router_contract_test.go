package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAssetRouterPublishesOnlyAssetsAndAssetGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetAssetRouter(engine)
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, expected := range []string{
		http.MethodPost + " /v1/assets",
		http.MethodGet + " /v1/assets/:asset_id",
		http.MethodPatch + " /v1/assets/:asset_id",
		http.MethodDelete + " /v1/assets/:asset_id",
		http.MethodPost + " /v1/asset-groups",
		http.MethodGet + " /v1/asset-groups/:group_id",
		http.MethodDelete + " /v1/asset-groups/:group_id",
	} {
		_, ok := routes[expected]
		assert.True(t, ok, expected)
	}

	for _, removed := range []string{
		http.MethodGet + " /v1/assets",
		http.MethodGet + " /v1/asset-groups",
		http.MethodGet + " /v1/api-service-rules/current",
		http.MethodPost + " /v1/api-service-rules/acceptance",
		http.MethodPost + " /v1/real-person-authorizations",
		http.MethodGet + " /verification/real-person/:token",
		http.MethodPost + " /v1/assets/:asset_id/migrations",
	} {
		_, ok := routes[removed]
		assert.False(t, ok, removed)
	}
}
