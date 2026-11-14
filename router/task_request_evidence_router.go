package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

const evidenceObjectRoute = "/:id/events/:event_id/object"

func apiGzip() gin.HandlerFunc {
	compress := gzip.Gzip(gzip.DefaultCompression)
	return func(c *gin.Context) {
		// Keep Content-Length on downloads so clients detect a short transfer.
		if c.FullPath() != "/api/task_request_evidence"+evidenceObjectRoute {
			compress(c)
		}
	}
}

// Admin/Root can explicitly read or download individual evidence objects.
// Cache prevention applies to the directory and all original content.
func registerTaskRequestEvidenceRoutes(apiRouter *gin.RouterGroup) {
	evidenceRoute := apiRouter.Group("/task_request_evidence", middleware.DisableCache())
	{
		evidenceRoute.GET("", middleware.AdminAuth(), controller.GetTaskRequestEvidenceList)
		evidenceRoute.GET("/:id", middleware.AdminAuth(), controller.GetTaskRequestEvidenceDetail)
		evidenceRoute.GET("/:id/events/:event_id/content", middleware.AdminAuth(), controller.GetTaskRequestEvidenceContent)
		evidenceRoute.GET(evidenceObjectRoute, middleware.AdminAuth(), controller.GetTaskRequestEvidenceObject)
	}
}
