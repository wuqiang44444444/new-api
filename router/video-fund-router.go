package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerVideoFundRoutes(api *gin.RouterGroup) {
	api.GET("/video-funds/self", middleware.UserAuth(), middleware.DisableCache(), controller.ListSelfVideoFundProgress)
	api.GET("/video-funds/token", middleware.TokenAuthReadOnly(), middleware.DisableCache(), controller.ListTokenVideoFundProgress)
	routes := api.Group("/video-funds", middleware.AdminAuth(), middleware.DisableCache())
	routes.GET("", controller.ListVideoFundLogs)
	routes.GET("/:kind/:id", controller.GetVideoFundLog)
	routes.POST("/:kind/:id/refund", middleware.RootAuth(), middleware.CriticalRateLimit(), controller.RefundVideoFunds)
}
