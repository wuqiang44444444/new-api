package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func registerArkPrefixCacheRoutes(router *gin.Engine) {
	router.DELETE("/v1/responses/:response_id", func(c *gin.Context) {
		id := c.Param("response_id")
		if len(id) > 64 || !model.IsArkManagedCacheID(id) {
			c.AbortWithStatus(http.StatusNotFound)
		}
	}, middleware.RouteTag("relay"), middleware.ArkCacheDeleteTokenAuth(), controller.DeleteArkPrefixCache)
}

func setTaskPluginProtocolRouterWithArkPilot(router *gin.Engine) {
	setTaskPluginProtocolRouter(router, middleware.ArkPrefixCacheTokenAuth(), controller.ArkPrefixCacheResponses)
}

func registerArkPrefixCacheBillingRoute(admin *gin.RouterGroup) {
	admin.POST("/ark-prefix-cache-hours", middleware.RootAuth(), controller.RecordArkPrefixCacheHour)
	admin.GET("/ark-cache-shadow-hours", middleware.RootAuth(), controller.ListArkCacheShadowHours)
	admin.DELETE("/ark-cache-responses/:response_id", middleware.RootAuth(), controller.DeleteArkCacheAsRoot)
}
