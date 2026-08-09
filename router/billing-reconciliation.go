package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func registerBillingReconciliationRoutes(apiRouter *gin.RouterGroup) {
	billingRoute := apiRouter.Group("/billing")
	billingRoute.GET("/statement/self", middleware.UserAuth(), controller.GetSelfBillingReconciliation)

	adminRoute := billingRoute.Group("/admin")
	adminRoute.Use(middleware.AdminAuth())
	{
		adminRoute.GET("/customer-statements", controller.GetAdminCustomerBillingStatements)
		adminRoute.GET("/customer-summary", controller.GetAdminCustomerBillingReconciliation)
		adminRoute.GET("/upstream-summary", controller.GetAdminUpstreamBillingReconciliation)
		adminRoute.PUT("/upstream-discounts", controller.PutAdminProviderBillingDiscount)
	}
}
