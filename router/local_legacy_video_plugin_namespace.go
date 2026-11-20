package router

import (
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
)

// These factory plugins have shipped static video routes under their namespace.
// Gin continues to serve those specific routes while the factory's other routes
// use the ordinary plugin dispatcher.
func localLegacyVideoPluginNamespace(key string, generation *jsplugin.RoutingGeneration) bool {
	return (key == "kling" || key == "jimeng") && generation.FactoryKey(key)
}

const localPluginFilePath = "/v1/files/retrieve"

// Azure Batch owns the parameterized file route. The literal MiniMax route
// takes priority in Gin and is handed to the upstream plugin dispatcher.
func setLocalPluginFileRoute(router *gin.Engine, dispatch gin.HandlerFunc) {
	router.GET(localPluginFilePath, middleware.RouteTag("relay"), dispatch, controller.RelayNotFound)
}

func (b *pluginGenerationBuilder) localPluginFileHandoff(route jsplugin.Route) bool {
	return route.Method == http.MethodGet && route.Path == localPluginFilePath &&
		slices.ContainsFunc(b.staticRoutes, func(host gin.RouteInfo) bool {
			return host.Method == http.MethodGet && host.Path == localPluginFilePath
		})
}
