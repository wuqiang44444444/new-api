package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerTaskContractRoutes(apiRouter *gin.RouterGroup) {
	route := apiRouter.Group("/task-contract")
	route.Use(middleware.RootAuth(), middleware.DisableCache())
	{
		route.GET("/attempts", controller.ListTaskCreateAttemptsForRecovery)
		route.POST(
			"/attempts/:attempt_id/recover",
			middleware.CriticalRateLimit(),
			controller.RecoverTaskCreateAttempt,
		)
		route.POST(
			"/attempts/:attempt_id/reject",
			middleware.CriticalRateLimit(),
			controller.RejectTaskCreateAttempt,
		)
	}
}
