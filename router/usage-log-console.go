package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// Older instances must reject these requests before running any legacy SELECT.
func registerUsageLogConsoleRoutes(logs *gin.RouterGroup) {
	logs.GET("/console/page", middleware.AdminAuth(), controller.GetAdminUsageLogConsolePage)
	logs.GET("/self/console/page", middleware.UserAuth(), controller.GetSelfUsageLogConsolePage)
	logs.GET("/console/stat", middleware.AdminAuth(), controller.GetAdminUsageLogConsoleStatistics)
	logs.GET("/self/console/stat", middleware.UserAuth(), controller.GetSelfUsageLogConsoleStatistics)
}
