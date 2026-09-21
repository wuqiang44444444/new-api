package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// registerUsageAnalyticsRoutes registers the read-only day/week usage analytics
// queries and the export submission endpoints. Admin routes re-check authority
// server-side; ordinary customers never see upstream fields.
func registerUsageAnalyticsRoutes(apiRouter *gin.RouterGroup) {
	usageRoute := apiRouter.Group("/usage")
	usageRoute.GET("/self/summary", middleware.UserAuth(), controller.GetUsageSelfSummary)
	usageRoute.GET("/admin/customers", middleware.AdminAuth(), controller.GetUsageAdminCustomers)
	usageRoute.GET("/admin/customer-summary", middleware.AdminAuth(), controller.GetUsageAdminCustomerSummary)
	usageRoute.GET("/admin/upstream-summary", middleware.AdminAuth(), controller.GetUsageAdminUpstreamSummary)
	usageRoute.POST("/self/exports", middleware.UserAuth(), controller.CreateUsageSelfExport)
	usageRoute.POST("/admin/exports", middleware.AdminAuth(), controller.CreateUsageAdminExport)
}
