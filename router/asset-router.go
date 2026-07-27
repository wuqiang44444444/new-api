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

	consent := router.Group("")
	consent.Use(middleware.IsolateConsentRoutes(), middleware.CriticalRateLimit())
	{
		consent.GET("/consent/real-person/verify/:token", controller.OpenRealPersonVerification)
		consent.GET("/consent/real-person/complete", controller.CompleteRealPersonConsent)
		consent.GET("/consent/real-person/receipt/:receipt_token", controller.ShowRealPersonReceipt)
		consent.GET("/consent/real-person/:token", controller.ShowRealPersonConsent)
		consent.POST("/api/real-person-consents/:token/accept", middleware.AnonymousRequestBodyLimit(), controller.AcceptRealPersonConsent)
		consent.POST("/api/real-person-consents/:token/reject", middleware.AnonymousRequestBodyLimit(), controller.RejectRealPersonConsent)
		consent.POST("/api/real-person-consents/receipt/:receipt_token/revoke", middleware.AnonymousRequestBodyLimit(), controller.RevokeRealPersonReceipt)
	}

	policyAdmin := router.Group("/api/asset-consent-policies")
	policyAdmin.Use(middleware.RouteTag("dashboard"), middleware.UserAuth(), middleware.AdminAuth(), middleware.CriticalRateLimit())
	{
		policyAdmin.GET("", controller.ListConsentPolicies)
		policyAdmin.POST("", controller.CreateConsentPolicy)
		policyAdmin.POST("/:policy_id/activate", controller.ActivateConsentPolicy)
	}
}
