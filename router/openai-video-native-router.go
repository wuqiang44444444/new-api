package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// registerOpenAIVideoNativeRoutes restores the upstream rc23 native video
// contract without routing registered Link SKUs through that compatibility API.
func registerOpenAIVideoNativeRoutes(router *gin.Engine) {
	legacyCreateRouter := router.Group("/v1")
	legacyCreateRouter.Use(middleware.RouteTag("relay"))
	legacyCreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol(model.TaskClientProtocolPlatformVideo),
		middleware.NativeOpenAIVideoContractConstraint(),
		middleware.AssetRouteConstraint(),
		middleware.Distribute(),
	)
	legacyCreateRouter.POST("/video/generations", controller.RelayTask)

	openAICreateRouter := router.Group("/v1")
	openAICreateRouter.Use(middleware.RouteTag("relay"))
	openAICreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol(model.TaskClientProtocolOpenAIVideos),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.NativeOpenAIVideoContractConstraint(),
		middleware.AssetRouteConstraint(),
		middleware.Distribute(),
	)
	openAICreateRouter.POST("/videos", controller.RelayTask)
	openAICreateRouter.POST("/videos/:video_id/remix", controller.RelayTask)
}
