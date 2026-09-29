package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// 音视频证据路由（一期）：管理员查看脱敏证据，Root 通过专用接口查看原文与下载
// 原始对象。整个路由组禁止缓存，原文查看与下载都要求 Root 权限。
func registerTaskRequestEvidenceRoutes(apiRouter *gin.RouterGroup) {
	evidenceRoute := apiRouter.Group("/task_request_evidence", middleware.DisableCache())
	{
		evidenceRoute.GET("", middleware.AdminAuth(), controller.GetTaskRequestEvidenceList)
		evidenceRoute.GET("/:id", middleware.AdminAuth(), controller.GetTaskRequestEvidenceDetail)
		evidenceRoute.GET("/:id/events/:event_id/content", middleware.RootAuth(), controller.GetTaskRequestEvidenceContent)
		evidenceRoute.GET("/:id/events/:event_id/object", middleware.RootAuth(), controller.GetTaskRequestEvidenceObject)
	}
}
