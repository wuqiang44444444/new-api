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
	billingRoute.GET("/statement/self/version-month-status", middleware.UserAuth(), controller.GetSelfBillingStatementVersionMonthStatus)
	billingRoute.GET("/statement/self/versions/:draft_public_id/lines", middleware.UserAuth(), controller.GetSelfBillingStatementVersionLines)
	billingRoute.GET("/statement/self/versions/:draft_public_id/download", middleware.UserAuth(), controller.GetSelfBillingStatementVersionDownload)

	// 客户异步导出：每一步重新校验发起人；下载只签发短时地址。
	exportRoute := billingRoute.Group("/exports")
	{
		exportRoute.POST("", middleware.UserAuth(), controller.CreateSelfCustomerExport)
		exportRoute.GET("", middleware.UserAuth(), controller.ListSelfCustomerExports)
		exportRoute.GET("/:job_id", middleware.UserAuth(), controller.GetSelfCustomerExport)
		exportRoute.POST("/:job_id/cancel", middleware.UserAuth(), controller.CancelSelfCustomerExport)
		exportRoute.GET("/:job_id/download", middleware.UserAuth(), controller.DownloadSelfCustomerExport)
	}

	adminRoute := billingRoute.Group("/admin")
	adminRoute.Use(middleware.AdminAuth())
	{
		adminRoute.GET("/customer-statements", controller.GetAdminCustomerBillingStatements)
		adminRoute.GET("/customer-logs", controller.GetAdminBillingStatementLogs)
		adminRoute.GET("/customer-logs/stat", controller.GetAdminBillingStatementLogs)
		adminRoute.GET("/customer-summary", controller.GetAdminCustomerBillingReconciliation)
		// 统一上游对账：URL 分组汇总 + 渠道月度折扣编辑 + 明细，替代原有两个上游 Tab。
		adminRoute.GET("/upstream-summary", controller.GetAdminUpstreamReconciliation)
		adminRoute.GET("/upstream-details", controller.GetAdminUpstreamBillingDetails)
		adminRoute.POST("/upstream-exports", controller.CreateAdminUpstreamExport)
		adminRoute.PUT("/upstream-discounts", controller.PutAdminProviderBillingDiscount)
		adminRoute.POST("/upstream-discounts/initialize", controller.PostAdminProviderChannelDiscountInit)
		adminRoute.PUT("/upstream-url-names", controller.PutAdminUpstreamURLGroupName)

		// 管理员代客导出：显式目标客户，限单客户；任务列表仍只看本人发起。
		adminExportRoute := adminRoute.Group("/customer-exports")
		{
			adminExportRoute.POST("", controller.CreateAdminCustomerExport)
			adminExportRoute.GET("/:job_id", controller.GetSelfCustomerExport)
			adminExportRoute.POST("/:job_id/cancel", controller.CancelSelfCustomerExport)
			adminExportRoute.GET("/:job_id/download", controller.DownloadSelfCustomerExport)
		}

		// 客户月账单版本固化：生成/确认/放弃/历史（docs/80-dev/2026-09-17 方案第 13 节）。
		adminRoute.GET("/customer-statement-source-review", controller.GetAdminBillingSourceReview)
		adminRoute.POST("/customer-statement-source-review", middleware.RootAuth(), controller.PostAdminBillingSourceReview)
		adminRoute.POST("/customer-statement-source-verification", middleware.RootAuth(), controller.PostAdminBillingStatementSourceVerification)
		adminRoute.POST("/customer-statement-versions", controller.PostAdminBillingStatementVersion)
		adminRoute.POST("/customer-statement-versions/correction", controller.PostAdminBillingStatementVersionCorrection)
		adminRoute.GET("/customer-statement-version-month-status", controller.GetAdminBillingStatementVersionMonthStatus)
		adminRoute.GET("/customer-statement-version-diff", controller.GetAdminBillingStatementVersionDiff)
		adminRoute.GET("/customer-statement-versions/:draft_public_id", controller.GetAdminBillingStatementVersion)
		adminRoute.GET("/customer-statement-versions/:draft_public_id/lines", controller.GetAdminBillingStatementVersionLines)
		adminRoute.GET("/customer-statement-versions/:draft_public_id/download", controller.GetAdminBillingStatementVersionDownload)
		adminRoute.POST("/customer-statement-versions/:draft_public_id/confirm", controller.PostAdminBillingStatementVersionConfirm)
		adminRoute.POST("/customer-statement-versions/:draft_public_id/abandon", controller.PostAdminBillingStatementVersionAbandon)
		adminRoute.POST("/customer-statement-versions/:draft_public_id/cleanup", controller.PostAdminBillingStatementVersionCleanup)
		adminRoute.GET("/customer-statement-version-history", controller.GetAdminBillingStatementVersionHistory)
	}
}
