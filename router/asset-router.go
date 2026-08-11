package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetAssetRouter(router *gin.Engine) {
	assets := router.Group("/v1")
	assets.Use(middleware.RouteTag("asset"), middleware.TokenAuth())
	{
		assets.POST("/assets", middleware.TokenModelAccess(), controller.CreateAsset)
		assets.GET("/assets", controller.ListAssets)
		assets.GET("/assets/:asset_id", controller.GetAsset)
		assets.PATCH("/assets/:asset_id", controller.UpdateAsset)
		assets.DELETE("/assets/:asset_id", controller.DeleteAsset)
		assets.POST("/asset-groups", middleware.TokenModelAccess(), controller.CreateAssetGroup)
		assets.GET("/asset-groups", controller.ListAssetGroups)
		assets.GET("/asset-groups/:group_id", controller.GetAssetGroup)
		assets.DELETE("/asset-groups/:group_id", controller.DeleteAssetGroup)
	}
}
