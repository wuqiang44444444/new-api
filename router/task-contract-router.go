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
		route.GET("/provider-exposures/metrics", controller.GetProviderExposureMetrics)
		route.GET("/provider-exposures/incidents", controller.ListProviderExposureIncidents)
		route.POST(
			"/provider-exposures/incidents/:id/resolve",
			middleware.CriticalRateLimit(),
			controller.ResolveProviderExposureIncident,
		)
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
