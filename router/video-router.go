package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	modelArkVideoCreateRouter := router.Group("/api/v3")
	modelArkVideoCreateRouter.Use(middleware.RouteTag("relay"))
	modelArkVideoCreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("modelark_v3"),
		middleware.TaskCreateResponseContract(),
		middleware.ModelArkVideoCreateConvert(),
		middleware.ResolveSeedanceChannel(),
	)
	{
		modelArkVideoCreateRouter.POST("/contents/generations/tasks", controller.RelayTask)
	}

	modelArkVideoReadRouter := router.Group("/api/v3")
	modelArkVideoReadRouter.Use(middleware.RouteTag("relay"))
	modelArkVideoReadRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"))
	{
		modelArkVideoReadRouter.GET("/contents/generations/models", controller.ListSeedanceModels)
		modelArkVideoReadRouter.GET("/contents/generations/tasks", controller.ModelArkVideoList)
		modelArkVideoReadRouter.GET("/contents/generations/tasks/:task_id", controller.ModelArkVideoGet)
		modelArkVideoReadRouter.DELETE("/contents/generations/tasks/:task_id", controller.ModelArkVideoDelete)
	}

	klingV1CreateRouter := router.Group("/kling/v1")
	klingV1CreateRouter.Use(middleware.RouteTag("relay"))
	klingV1CreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("kling_v1"),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.KlingRequestConvert(),
		middleware.Distribute(),
	)
	{
		klingV1CreateRouter.POST("/videos/text2video", controller.RelayTask)
		klingV1CreateRouter.POST("/videos/image2video", controller.RelayTask)
	}

	klingV1ReadRouter := router.Group("/kling/v1")
	klingV1ReadRouter.Use(middleware.RouteTag("relay"))
	klingV1ReadRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("kling_v1"))
	{
		klingV1ReadRouter.GET("/videos/text2video/:task_id", controller.KlingVideoGet)
		klingV1ReadRouter.GET("/videos/image2video/:task_id", controller.KlingVideoGet)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("jimeng_official"),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.JimengRequestConvert(),
		middleware.Distribute(),
	)
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.JimengVideo)
	}
}
