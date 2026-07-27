package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAssetRouterDoesNotExposeUnsupportedManagedGroupContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetAssetRouter(engine)
	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, expected := range []string{
		http.MethodPost + " /v1/asset-groups",
		http.MethodGet + " /v1/asset-groups",
		http.MethodGet + " /v1/asset-groups/:group_id",
		http.MethodPatch + " /v1/asset-groups/:group_id",
		http.MethodDelete + " /v1/asset-groups/:group_id",
	} {
		_, ok := routes[expected]
		assert.False(t, ok, expected)
	}
}

func TestAssetRouterExposesNewResourceMigrationWithoutPublicGroupCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetAssetRouter(engine)
	routes := map[string]struct{}{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, exists := routes[http.MethodPost+" /v1/assets/:asset_id/migrations"]
	assert.True(t, exists)
}
