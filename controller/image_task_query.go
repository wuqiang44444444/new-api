package controller

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// 显式图片任务的查询投影（§3.7/§5）。复用 GET /v1/tasks/{task_id}，仅对
// image_openai_v1 任务返回图片形状；授权按 user_id + app_id 双重归属，
// 不以摘要代替授权。结果 URL 逐张 300 秒签名并给出到期时间，过期后再次
// 授权查询续签；显式 b64_json 逐张读取对象原文返回。逐图区分可用/已删除/
// 暂不可用：已删除保留历史生成状态，不抹去其它图片（评审 S12）。

type imageTaskQueryDataItem struct {
	URL          string `json:"url,omitempty"`
	URLExpiresAt int64  `json:"url_expires_at,omitempty"`
	B64JSON      string `json:"b64_json,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	Status       string `json:"status"` // available | deleted | unavailable
}

// projectImageTaskQuery writes the image task projection. It assumes the
// caller already validated user ownership; app scope is re-checked here.
func projectImageTaskQuery(c *gin.Context, taskID string) bool {
	userID := c.GetInt("id")
	appID := c.GetInt("token_id")
	task, exists, err := model.GetByTaskIDForApp(userID, appID, taskID)
	if err != nil {
		imageTaskQueryError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return true
	}
	if !exists || task == nil || !model.IsImageTask(task) {
		// 归属不匹配与其不存在不可区分。
		imageTaskQueryError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return true
	}

	data := task.PrivateData.ImageTask
	response := gin.H{
		"id":         task.TaskID,
		"object":     "image_task",
		"status":     imageTaskQueryStatus(task.Status),
		"created_at": taskCreatedAt(task),
	}
	if task.FinishTime != 0 {
		response["finished_at"] = task.FinishTime
	}
	if data != nil && data.ImageCount > 0 {
		response["image_count"] = data.ImageCount
	}
	if task.Status == model.TaskStatusFailure || task.Status == model.TaskStatusExpired {
		response["error"] = gin.H{
			"message": model.SanitizeImageTaskForClientError(dataFailureCode(data)),
			"code":    dataFailureCode(data),
		}
	}
	if task.Status == model.TaskStatusReconciliationRequired {
		response["error"] = gin.H{
			"message": "the image task outcome is pending verification",
			"code":    dataFailureCode(data),
		}
	}

	if data != nil && len(data.Artifacts) > 0 {
		items := make([]imageTaskQueryDataItem, 0, len(data.Artifacts))
		wantB64 := strings.EqualFold(data.ResponseFormat, "b64_json")
		headCtx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
		defer cancel()
		headCtx, storeErr := service.WithImageObjectStore(headCtx)
		for _, artifact := range data.Artifacts {
			if storeErr != nil {
				items = append(items, imageTaskQueryDataItem{MimeType: artifact.MimeType, Status: "unavailable"})
				continue
			}
			items = append(items, projectImageArtifact(headCtx, artifact, wantB64))
		}
		response["data"] = items
	}

	c.JSON(http.StatusOK, response)
	return true
}

// projectImageArtifact 逐图投影：HEAD 404 → deleted（保留历史事实）；
// 其它探测/读取错误 → unavailable（暂不可用，不判删除，不伪装可交付）；
// 只有可用图片签发 300 秒 URL 或返回 b64 原文。
func projectImageArtifact(ctx context.Context, artifact model.TaskImageArtifact, wantB64 bool) imageTaskQueryDataItem {
	item := imageTaskQueryDataItem{MimeType: artifact.MimeType, Status: "available"}
	headCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	exists, err := service.HeadImageObject(headCtx, artifact.ObjectKey)
	cancel()
	if err != nil {
		item.Status = "unavailable"
		return item
	}
	if !exists {
		item.Status = "deleted"
		return item
	}
	if wantB64 {
		content, err := service.FetchImageObjectBytes(ctx, artifact.ObjectKey)
		if err != nil {
			item.Status = "unavailable"
			return item
		}
		item.B64JSON = base64.StdEncoding.EncodeToString(content)
		return item
	}
	url, expiresAt, err := service.PresignImageObjectURL(ctx, artifact.ObjectKey)
	if err != nil {
		item.Status = "unavailable"
		return item
	}
	item.URL = url
	item.URLExpiresAt = expiresAt
	return item
}

func imageTaskQueryStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusNotStart, model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	case model.TaskStatusInProgress:
		return "in_progress"
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusExpired:
		return "expired"
	case model.TaskStatusReconciliationRequired, model.TaskStatusUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func taskCreatedAt(task *model.Task) int64 {
	if task.CreatedAt != 0 {
		return task.CreatedAt
	}
	return task.SubmitTime
}

func dataFailureCode(data *model.TaskImageExecutionData) string {
	if data == nil {
		return ""
	}
	return data.FailureCode
}

func imageTaskQueryError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"code":    errType,
		},
	})
}
