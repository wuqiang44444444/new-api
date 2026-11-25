package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// Register only published Batch methods; ownership and channel selection stay
// in the typed service.
func registerBatchRelayRoutes(parent *gin.RouterGroup) {
	batchRouter := parent.Group("", middleware.ModelRequestRateLimit())
	{
		batchRouter.POST("/batches", controller.CreateBatch)
		batchRouter.POST("/files", controller.UploadBatchFile)
		batchRouter.GET("/files", controller.ListBatchFiles)
		batchRouter.GET("/files/:id", controller.RetrieveBatchFile)
		batchRouter.GET("/files/:id/content", controller.DownloadBatchFile)
		batchRouter.GET("/batches", controller.ListBatches)
		batchRouter.GET("/batches/:id", controller.RetrieveBatch)
		batchRouter.POST("/batches/:id/cancel", controller.CancelBatch)
	}

}
