package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type openAIVideoListResponse struct {
	Object  string             `json:"object"`
	Data    []*dto.OpenAIVideo `json:"data"`
	FirstID *string            `json:"first_id"`
	LastID  *string            `json:"last_id"`
	HasMore bool               `json:"has_more"`
}

func OpenAIVideoList(c *gin.Context) {
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			openAIVideoError(c, http.StatusBadRequest, "invalid_request_error", "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = value
	}
	order := strings.TrimSpace(c.Query("order"))
	if order == "" {
		order = "desc"
	}
	if order != "asc" && order != "desc" {
		openAIVideoError(c, http.StatusBadRequest, "invalid_request_error", "invalid_order", "order must be asc or desc")
		return
	}
	tasks, hasMore, err := model.ListOpenAIVideoTasks(c.GetInt("id"), c.GetInt("token_id"), strings.TrimSpace(c.Query("after")), limit, order)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			openAIVideoError(c, http.StatusBadRequest, "invalid_request_error", "invalid_after", "after does not identify a visible video")
		} else {
			openAIVideoError(c, http.StatusInternalServerError, "server_error", "internal_error", "Failed to list videos")
		}
		return
	}
	response := openAIVideoListResponse{Object: "list", Data: make([]*dto.OpenAIVideo, 0, len(tasks)), HasMore: hasMore}
	for i := range tasks {
		response.Data = append(response.Data, tasks[i].ToOpenAIVideo())
	}
	if len(response.Data) > 0 {
		response.FirstID = common.GetPointer(response.Data[0].ID)
		response.LastID = common.GetPointer(response.Data[len(response.Data)-1].ID)
	}
	c.JSON(http.StatusOK, response)
}

func OpenAIVideoGet(c *gin.Context) {
	task, exists, err := model.GetVideoTaskForProtocol(c.GetInt("id"), c.GetInt("token_id"), c.Param("task_id"), model.TaskClientProtocolOpenAIVideos, false)
	if err != nil {
		openAIVideoError(c, http.StatusInternalServerError, "server_error", "internal_error", "Failed to retrieve video")
		return
	}
	if !exists {
		openAIVideoError(c, http.StatusNotFound, "invalid_request_error", "video_not_found", "Video not found")
		return
	}
	c.JSON(http.StatusOK, task.ToOpenAIVideo())
}

func OpenAIVideoDelete(c *gin.Context) {
	task, exists, err := model.GetVideoTaskForProtocol(c.GetInt("id"), c.GetInt("token_id"), c.Param("task_id"), model.TaskClientProtocolOpenAIVideos, false)
	if err != nil {
		openAIVideoError(c, http.StatusInternalServerError, "server_error", "internal_error", "Failed to delete video")
		return
	}
	if !exists {
		openAIVideoError(c, http.StatusNotFound, "invalid_request_error", "video_not_found", "Video not found")
		return
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure &&
		task.Status != model.TaskStatusCancelled && task.Status != model.TaskStatusExpired {
		openAIVideoError(c, http.StatusConflict, "invalid_request_error", "video_not_terminal", "Only completed or failed videos can be deleted")
		return
	}
	providerChannel, err := videoTaskProviderChannel(task)
	if err != nil {
		openAIVideoError(c, http.StatusServiceUnavailable, "server_error", "upstream_unavailable", "Video storage is temporarily unavailable")
		return
	}
	if err := relay.DeleteTerminalVideoTask(c.Request.Context(), task, providerChannel); err != nil {
		openAIVideoError(c, http.StatusBadGateway, "server_error", "upstream_unavailable", "Video storage is temporarily unavailable")
		return
	}
	deleted, err := model.MarkVideoTaskClientDeleted(c.GetInt("id"), c.GetInt("token_id"), task.TaskID, model.TaskClientProtocolOpenAIVideos)
	if err != nil || !deleted {
		openAIVideoError(c, http.StatusConflict, "invalid_request_error", "video_delete_conflict", "Video deletion could not be completed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": task.TaskID, "deleted": true, "object": "video.deleted"})
}

func openAIVideoError(c *gin.Context, status int, errorType, code, message string) {
	requestID := c.GetString(common.RequestIdKey)
	c.JSON(status, gin.H{"error": gin.H{
		"message": common.MessageWithRequestId(message, requestID),
		"type":    errorType,
		"param":   nil,
		"code":    code,
	}})
}
