package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
)

// Register only published Batch methods; ownership and channel selection stay
// in the typed service. The parent supplies token authentication.
func registerBatchRelayRoutes(parent *gin.RouterGroup) {
	batchRouter := parent.Group("")
	{
		batchRouter.POST("/files", controller.UploadBatchFile)
		batchRouter.GET("/files", controller.ListBatchFiles)
		batchRouter.GET("/files/:id", controller.RetrieveBatchFile)
		batchRouter.GET("/files/:id/content", controller.DownloadBatchFile)
		batchRouter.POST("/batches", controller.CreateBatch)
		batchRouter.GET("/batches", controller.ListBatches)
		batchRouter.GET("/batches/:id", controller.RetrieveBatch)
		batchRouter.POST("/batches/:id/cancel", controller.CancelBatch)
	}

}
