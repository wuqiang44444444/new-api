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
		assets.GET("/api-service-rules/current", controller.GetCurrentAPIServiceRule)
		assets.POST("/api-service-rules/acceptance", controller.AcceptCurrentAPIServiceRule)
		assets.POST("/assets", middleware.TokenModelAccess(), controller.CreateAsset)
		assets.POST("/assets/:asset_id/migrations", middleware.TokenModelAccess(), controller.MigrateAsset)
		assets.GET("/assets", controller.ListAssets)
		assets.GET("/assets/:asset_id", controller.GetAsset)
		assets.PATCH("/assets/:asset_id", controller.UpdateAsset)
		assets.DELETE("/assets/:asset_id", controller.DeleteAsset)
		assets.GET("/assets/:asset_id/bindings", controller.ListAssetBindings)
		assets.POST("/real-person-authorizations", middleware.TokenModelAccess(), controller.CreateRealPersonAuthorization)
		assets.GET("/real-person-authorizations/:authorization_id", controller.GetRealPersonAuthorization)
		assets.POST("/real-person-authorizations/:authorization_id/revoke", controller.RevokeRealPersonAuthorization)
		assets.POST("/real-person-authorizations/:authorization_id/retry", controller.RetryRealPersonAuthorization)
	}

	verification := router.Group("")
	verification.Use(middleware.IsolateConsentRoutes(), middleware.CriticalRateLimit())
	{
		verification.HEAD("/verification/real-person/:token", controller.CheckRealPersonVerification)
		verification.GET("/verification/real-person/:token", controller.OpenRealPersonVerification)
		verification.GET("/verification/real-person/complete", controller.CompleteRealPersonVerification)
	}

	ruleAdmin := router.Group("/api/api-service-rules")
	ruleAdmin.Use(middleware.RouteTag("dashboard"), middleware.UserAuth(), middleware.AdminAuth(), middleware.CriticalRateLimit())
	{
		ruleAdmin.GET("", controller.ListAPIServiceRules)
		ruleAdmin.POST("", controller.CreateAPIServiceRule)
		ruleAdmin.GET("/acceptances", controller.ListAPIServiceRuleAcceptances)
		ruleAdmin.POST("/:rule_id/activate", controller.ActivateAPIServiceRule)
	}
}
