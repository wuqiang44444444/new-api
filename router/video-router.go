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
	legacyVideoRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("platform_video"))
	{
		legacyVideoRouter.GET("/video/generations/:task_id", controller.RelayTaskFetch)
	}

	// OpenAI video models are retired. Read-only routes remain temporarily for
	// historical tasks; no route can create or remix an OpenAI video task.
	openAIVideoReadRouter := router.Group("/v1")
	openAIVideoReadRouter.Use(middleware.RouteTag("relay"))
	openAIVideoReadRouter.Use(middleware.TokenAuth(), middleware.TaskClientProtocol("openai_videos"))
	{
		openAIVideoReadRouter.GET("/videos", controller.OpenAIVideoList)
		openAIVideoReadRouter.GET("/videos/:task_id", controller.OpenAIVideoGet)
	}

	modelArkVideoCreateRouter := router.Group("/api/v3")
	modelArkVideoCreateRouter.Use(middleware.RouteTag("relay"))
	modelArkVideoCreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("modelark_v3"),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.ModelArkVideoCreateConvert(),
		middleware.ResolveVideoSKUCapability(),
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

	klingV1CreateRouter := router.Group("/kling/v1")
	klingV1CreateRouter.Use(middleware.RouteTag("relay"))
	klingV1CreateRouter.Use(
		middleware.TokenAuth(),
		middleware.TaskClientProtocol("kling_v1"),
		middleware.TaskCreateResponseContract(),
		middleware.TaskCreateIdempotency(),
		middleware.KlingRequestConvert(),
		middleware.ResolveVideoSKUCapability(),
		middleware.AssetRouteConstraint(),
		middleware.VideoSKUChannelConstraint(),
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
		middleware.ResolveVideoSKUCapability(),
		middleware.AssetRouteConstraint(),
		middleware.VideoSKUChannelConstraint(),
		middleware.Distribute(),
	)
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.JimengVideo)
	}
}
