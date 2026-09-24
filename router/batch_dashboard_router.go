package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func setBatchDashboardRouter(group *gin.RouterGroup) {
	group.GET("/batch/:id/billing/calculation", middleware.UserAuth(), controller.BatchLineCalculation)
	group.GET("/batch/:id/billing", middleware.UserAuth(), controller.BatchBillingDetails)
}
