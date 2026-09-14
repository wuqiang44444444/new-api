package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func registerBillingReconciliationRoutes(apiRouter *gin.RouterGroup) {
	billingRoute := apiRouter.Group("/billing")
	billingRoute.GET("/statement/self", middleware.UserAuth(), controller.GetSelfBillingReconciliation)
	billingRoute.GET("/statement/self/logs", middleware.UserAuth(), controller.GetSelfBillingStatementLogs)
	billingRoute.GET("/statement/self/logs/stat", middleware.UserAuth(), controller.GetSelfBillingStatementLogs)

	adminRoute := billingRoute.Group("/admin")
	adminRoute.Use(middleware.AdminAuth())
	{
		adminRoute.GET("/customer-statements", controller.GetAdminCustomerBillingStatements)
		adminRoute.GET("/customer-logs", controller.GetAdminBillingStatementLogs)
		adminRoute.GET("/customer-logs/stat", controller.GetAdminBillingStatementLogs)
		adminRoute.GET("/customer-summary", controller.GetAdminCustomerBillingReconciliation)
		adminRoute.GET("/upstream-summary", controller.GetAdminUpstreamBillingReconciliation)
		adminRoute.GET("/upstream-url-summary", controller.GetAdminUpstreamBillingURLReconciliation)
		adminRoute.PUT("/upstream-discounts", controller.PutAdminProviderBillingDiscount)
	}
}
