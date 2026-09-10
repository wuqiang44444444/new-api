package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func respondBatchInvalid(c *gin.Context, err error) {
	var validateErr *dto.BatchValidateError
	if errors.As(err, &validateErr) {
		status := http.StatusBadRequest
		body := gin.H{"success": false, "error": gin.H{
			"message": validateErr.Message, "type": "invalid_request_error",
		}}
		if validateErr.Line > 0 {
			body["error"].(gin.H)["line"] = validateErr.Line
		}
		c.JSON(status, body)
		return
	}
	if errors.Is(err, service.ErrBatchJobRejected) || errors.Is(err, service.ErrBatchJobModelDenied) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{
			"message": err.Error(), "type": "invalid_request_error",
		}})
		return
	}
	if errors.Is(err, service.ErrBatchObjectNotFound) || errors.Is(err, model.ErrBatchFileNotFound) || errors.Is(err, model.ErrBatchJobNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{
			"message": "the requested batch resource does not exist", "type": "invalid_request_error",
		}})
		return
	}
	logger.LogError(c.Request.Context(), "batch request failed: "+common.SanitizeTaskDiagnostic(err.Error()))
	if errors.Is(err, service.ErrBatchObjectStoreUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{
			"message": "batch file storage is temporarily unavailable", "type": "service_unavailable",
		}})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": gin.H{
		"message": "batch processing is temporarily unavailable", "type": "service_unavailable",
	}})
}

// UploadBatchFile handles POST /v1/files with purpose=batch (multipart).
func UploadBatchFile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, dto.MaxBatchFileBytes+(1<<20))
	defer func() {
		if c.Request.MultipartForm != nil {
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()
	purpose := c.PostForm("purpose")
	header, err := c.FormFile("file")
	if err != nil {
		respondBatchInvalid(c, &dto.BatchValidateError{Message: "a multipart file field is required"})
		return
	}
	file, openErr := header.Open()
	if openErr != nil {
		respondBatchInvalid(c, &dto.BatchValidateError{Message: "the uploaded file could not be read"})
		return
	}
	defer file.Close()
	result, err := service.UploadBatchFile(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), header.Filename, purpose, file)
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	c.JSON(http.StatusOK, service.BatchFileObjectView(result.File))
}

// ListBatchFiles handles GET /v1/files.
func ListBatchFiles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, err := model.ListBatchFilesOwned(c.GetInt("id"), c.GetInt("token_id"), limit, c.Query("after"))
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	items := make([]dto.BatchFileObject, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, *service.BatchFileObjectView(&page.Items[i]))
	}
	first, last := "", ""
	if len(items) > 0 {
		first, last = items[0].Id, items[len(items)-1].Id
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items, "total": page.Total, "has_more": page.HasMore, "first_id": first, "last_id": last})
}

// RetrieveBatchFile handles GET /v1/files/:id.
func RetrieveBatchFile(c *gin.Context) {
	file, err := service.RetrieveBatchFile(c.Param("id"), c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	c.JSON(http.StatusOK, service.BatchFileObjectView(file))
}

// DownloadBatchFile handles GET /v1/files/:id/content.
func DownloadBatchFile(c *gin.Context) {
	c.Header("Content-Type", "application/jsonl")
	_, _, err := service.BatchFileContent(c.Request.Context(), c.Param("id"), c.GetInt("id"), c.GetInt("token_id"), c.Writer)
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
}

// CreateBatch handles POST /v1/batches.
func CreateBatch(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var request dto.BatchCreateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondBatchInvalid(c, &dto.BatchValidateError{Message: "a valid JSON request body is required"})
		return
	}
	result, err := service.CreateBatchJob(c, &request)
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	c.JSON(http.StatusOK, service.BatchJobObjectView(result.Job))
}

// ListBatches handles GET /v1/batches.
func ListBatches(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, err := model.ListBatchJobsOwned(c.GetInt("id"), c.GetInt("token_id"), limit, c.Query("after"))
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	items := make([]dto.BatchJobObject, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, *service.BatchJobObjectView(&page.Items[i]))
	}
	first, last := "", ""
	if len(items) > 0 {
		first, last = items[0].Id, items[len(items)-1].Id
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items, "total": page.Total, "has_more": page.HasMore, "first_id": first, "last_id": last})
}

// RetrieveBatch handles GET /v1/batches/:id.
func RetrieveBatch(c *gin.Context) {
	job, err := model.GetBatchJobOwned(c.Param("id"), c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	c.JSON(http.StatusOK, service.BatchJobObjectView(job))
}

// CancelBatch handles POST /v1/batches/:id/cancel.
func CancelBatch(c *gin.Context) {
	job, err := service.CancelBatchJob(c.Param("id"), c.GetInt("id"), c.GetInt("token_id"))
	if err != nil {
		respondBatchInvalid(c, err)
		return
	}
	c.JSON(http.StatusOK, service.BatchJobObjectView(job))
}
