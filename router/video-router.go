package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	legacyVideoRouter := router.Group("/v1")
	legacyVideoRouter.Use(middleware.RouteTag("relay"))
	legacyVideoRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("platform_video"), middleware.AssetRouteConstraint(), middleware.Distribute())
	{
		legacyVideoRouter.POST("/video/generations", controller.RelayTask)
		legacyVideoRouter.GET("/video/generations/:task_id", controller.RelayTaskFetch)
	}

	openAIVideoCreateRouter := router.Group("/v1")
	openAIVideoCreateRouter.Use(middleware.RouteTag("relay"))
	openAIVideoCreateRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("openai_videos"), middleware.TaskCreateResponseContract(), middleware.TaskCreateIdempotency(), middleware.AssetRouteConstraint(), middleware.Distribute())
	{
		openAIVideoCreateRouter.POST("/videos", controller.RelayTask)
		openAIVideoCreateRouter.POST("/videos/:video_id/remix", controller.RelayTask)
	}

	openAIVideoReadRouter := router.Group("/v1")
	openAIVideoReadRouter.Use(middleware.RouteTag("relay"))
	openAIVideoReadRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("openai_videos"))
	{
		openAIVideoReadRouter.GET("/videos", controller.OpenAIVideoList)
		openAIVideoReadRouter.GET("/videos/:task_id", controller.OpenAIVideoGet)
		openAIVideoReadRouter.DELETE("/videos/:task_id", controller.OpenAIVideoDelete)
	}

	modelArkVideoCreateRouter := router.Group("/api/v3")
	modelArkVideoCreateRouter.Use(middleware.RouteTag("relay"))
	modelArkVideoCreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("modelark_v3"),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.ModelArkVideoCreateConvert(),
		middleware.AssetRouteConstraint(),
		middleware.ModelArkVideoChannelConstraint(),
		middleware.Distribute(),
	)
	{
		modelArkVideoCreateRouter.POST("/contents/generations/tasks", controller.RelayTask)
	}

	modelArkVideoReadRouter := router.Group("/api/v3")
	modelArkVideoReadRouter.Use(middleware.RouteTag("relay"))
	modelArkVideoReadRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("modelark_v3"))
	{
		modelArkVideoReadRouter.GET("/contents/generations/tasks", controller.ModelArkVideoList)
		modelArkVideoReadRouter.GET("/contents/generations/tasks/:task_id", controller.ModelArkVideoGet)
		modelArkVideoReadRouter.DELETE("/contents/generations/tasks/:task_id", controller.ModelArkVideoDelete)
	}

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.TaskClientProtocol("platform_video"), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.TaskClientProtocol("platform_video"), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}
}
