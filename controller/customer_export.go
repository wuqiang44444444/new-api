package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 客户导出控制台接口（10.3）：创建、列表、详情、取消、下载。每一步都重新
// 校验发起人；管理员接口另校验目标客户。列表、详情和进度只读任务小表，
// 不重新扫描日志；下载签发短时地址，签名 URL 不落日志。

type customerExportHttpRequest struct {
	JobType           string `json:"job_type"`
	StartTimestamp    int64  `json:"start_timestamp"`
	EndTimestamp      int64  `json:"end_timestamp"`
	LogTypes          []int  `json:"log_types,omitempty"`
	TokenId           *int   `json:"token_id,omitempty"`
	ChannelId         *int   `json:"channel_id,omitempty"`
	TokenName         string `json:"token_name,omitempty"`
	Group             string `json:"group,omitempty"`
	RequestId         string `json:"request_id,omitempty"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty"`
	Username          string `json:"username,omitempty"`
	ModelName         string `json:"model_name,omitempty"`
	BillingMode       string `json:"billing_mode,omitempty"`
	Language          string `json:"language,omitempty"`
}

func parseCustomerExportRequest(c *gin.Context) (service.CustomerExportRequest, bool) {
	var request customerExportHttpRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "invalid export request")
		return service.CustomerExportRequest{}, false
	}
	return service.CustomerExportRequest{
		JobType:           request.JobType,
		StartTimestamp:    request.StartTimestamp,
		EndTimestamp:      request.EndTimestamp,
		LogTypes:          request.LogTypes,
		TokenId:           request.TokenId,
		ChannelId:         request.ChannelId,
		TokenName:         request.TokenName,
		Group:             request.Group,
		RequestId:         request.RequestId,
		UpstreamRequestId: request.UpstreamRequestId,
		Username:          request.Username,
		ModelName:         request.ModelName,
		BillingMode:       request.BillingMode,
		Language:          request.Language,
	}, true
}

func CreateSelfCustomerExport(c *gin.Context) {
	request, ok := parseCustomerExportRequest(c)
	if !ok {
		return
	}
	job, err := service.SubmitCustomerExportJob(c.GetInt("id"), c.GetInt("id"), request)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": job.ToView()})
}

func CreateAdminCustomerExport(c *gin.Context) {
	request, ok := parseCustomerExportRequest(c)
	if !ok {
		return
	}
	var userId int
	var err error
	if raw := c.Query("user_id"); raw != "" {
		userId, err = strconv.Atoi(raw)
	} else if username := strings.TrimSpace(c.Query("username")); username != "" {
		userId, err = model.ResolveCustomerExportUserId(c.Request.Context(), username)
	}
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user_id")
		return
	}
	if _, err := model.GetBillingReconciliationUserById(userId); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "user not found"})
			return
		}
		respondCustomerExportError(c, err)
		return
	}
	job, err := service.SubmitCustomerExportJob(c.GetInt("id"), userId, request)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": job.ToView()})
}

func ListSelfCustomerExports(c *gin.Context) {
	jobs, err := model.ListCustomerExportJobs(c.GetInt("id"), 50)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	views := make([]model.CustomerExportJobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, job.ToView())
	}
	common.ApiSuccess(c, gin.H{"items": views})
}

func GetSelfCustomerExport(c *gin.Context) {
	job, ok := loadOwnedCustomerExport(c)
	if !ok {
		return
	}
	common.ApiSuccess(c, job.ToView())
}

func CancelSelfCustomerExport(c *gin.Context) {
	job, ok := loadOwnedCustomerExport(c)
	if !ok {
		return
	}
	job, err := model.CancelCustomerExportJob(job.JobID, c.GetInt("id"))
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	common.ApiSuccess(c, job.ToView())
}

// DownloadSelfCustomerExport 在下载前重新鉴权并签发短时地址；地址有效期
// 不超过产物剩余有效期。签名 URL 只出现在本次响应中。
func DownloadSelfCustomerExport(c *gin.Context) {
	job, ok := loadOwnedCustomerExport(c)
	if !ok {
		return
	}
	respondCustomerExportDownload(c, job)
}

func loadOwnedCustomerExport(c *gin.Context) (*model.CustomerExportJob, bool) {
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		common.ApiErrorMsg(c, "invalid job id")
		return nil, false
	}
	job, err := model.GetCustomerExportJobForOwner(jobID, c.GetInt("id"))
	if err != nil {
		respondCustomerExportError(c, err)
		return nil, false
	}
	return job, true
}

func respondCustomerExportDownload(c *gin.Context, job *model.CustomerExportJob) {
	if job.Status != model.CustomerExportJobStatusSucceeded || job.ExpiresAt <= 0 || job.ExpiresAt <= common.GetTimestamp() {
		common.ApiErrorMsg(c, "export file is no longer available")
		return
	}
	artifact := job.DecodeArtifact()
	if artifact == nil || len(artifact.Files) == 0 {
		common.ApiSuccess(c, gin.H{"files": []any{}, "empty_result": true, "generated_at": artifactGeneratedAt(artifact)})
		return
	}
	ttl := time.Duration(artifact.ExpiresAt-common.GetTimestamp()) * time.Second
	if ttl > customerExportDownloadTTL {
		ttl = customerExportDownloadTTL
	}
	files := make([]gin.H, 0, len(artifact.Files))
	for _, file := range artifact.Files {
		url, expiresAt, err := signCustomerExportDownloadURL(file.ObjectKey, ttl, artifact.StoreIdentity)
		if err != nil {
			common.ApiErrorMsg(c, "unable to issue download address")
			return
		}
		files = append(files, gin.H{
			"file_name": file.FileName, "url": url, "expires_at": expiresAt,
			"size_bytes": file.SizeBytes, "line_count": file.LineCount, "sha256": file.Sha256,
		})
	}
	common.ApiSuccess(c, gin.H{"files": files, "empty_result": false, "generated_at": artifactGeneratedAt(artifact)})
}

func artifactGeneratedAt(artifact *model.CustomerExportArtifact) int64 {
	if artifact == nil {
		return 0
	}
	return artifact.GeneratedAt
}

var customerExportDownloadTTL = 5 * time.Minute

// signCustomerExportDownloadURL 经由导出存储能力签发短时 GET 地址。
func signCustomerExportDownloadURL(objectKey string, ttl time.Duration, identity string) (string, int64, error) {
	return service.PresignCustomerExportURL(objectKey, ttl, identity)
}

func respondCustomerExportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrCustomerExportUserBusy):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "You already have an active export job. Wait for it to finish or cancel it."})
	case errors.Is(err, model.ErrCustomerExportQueueBusy):
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Export service is busy. Please try again later."})
	case errors.Is(err, model.ErrCustomerExportNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "export job not found"})
	case errors.Is(err, model.ErrCustomerExportNotCancellable):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "export job cannot be cancelled"})
	case errors.Is(err, model.ErrCustomerExportBackendUnsupported):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Export is not available for this log database backend."})
	case errors.Is(err, service.ErrCustomerExportStorageUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Export requires object storage to be configured."})
	case errors.Is(err, service.ErrCustomerExportInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "unable to process customer export"})
	}
}
