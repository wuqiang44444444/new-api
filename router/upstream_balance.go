package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerUpstreamBalanceRoutes(apiRouter *gin.RouterGroup) {
	routes := apiRouter.Group("/upstream-balances", middleware.AdminAuth(), middleware.RequirePermission(authz.ChannelRead), middleware.RequirePermission(authz.ChannelOperate))
	routes.GET("/", controller.GetUpstreamBalanceConnections)
	routes.GET("/:id/:key_index", controller.GetUpstreamBalance)
}
