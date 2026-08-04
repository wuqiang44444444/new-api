package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func KlingVideoGet(c *gin.Context) {
	task, exists, err := videoTaskForProtocol(
		c.GetInt("id"),
		c.Param("task_id"),
		model.TaskClientProtocolKlingV1,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.KlingVideoErrorResponse{
			Code: dto.KlingVideoErrorCode(http.StatusInternalServerError), Message: "Failed to load video task",
			RequestID: c.GetString(common.RequestIdKey), Data: nil,
		})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, dto.KlingVideoErrorResponse{
			Code: dto.KlingVideoErrorCode(http.StatusNotFound), Message: "Video task was not found",
			RequestID: c.GetString(common.RequestIdKey), Data: nil,
		})
		return
	}
	result := gin.H{
		"task_id":         task.TaskID,
		"task_status":     klingTaskStatus(task.Status),
		"task_status_msg": task.FailReason,
		"created_at":      task.CreatedAt,
		"updated_at":      task.UpdatedAt,
	}
	if task.Status == model.TaskStatusSuccess && task.GetResultURL() != "" {
		result["task_result"] = gin.H{"videos": []gin.H{{"url": task.GetResultURL()}}}
	}
	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"message":    "SUCCEED",
		"request_id": c.GetString(common.RequestIdKey),
		"data":       result,
	})
}

func JimengVideo(c *gin.Context) {
	if c.GetString("task_id") == "" {
		RelayTask(c)
		return
	}
	task, exists, err := videoTaskForProtocol(
		c.GetInt("id"),
		c.GetString("task_id"),
		model.TaskClientProtocolJimeng,
	)
	if err != nil {
		code := dto.JimengVideoErrorCode(http.StatusInternalServerError)
		c.JSON(http.StatusInternalServerError, dto.JimengVideoErrorResponse{
			Code: code, Data: nil, Message: "Failed to load video task",
			RequestID: c.GetString(common.RequestIdKey), Status: code,
		})
		return
	}
	if !exists {
		code := dto.JimengVideoErrorCode(http.StatusNotFound)
		c.JSON(http.StatusNotFound, dto.JimengVideoErrorResponse{
			Code: code, Data: nil, Message: "Video task was not found",
			RequestID: c.GetString(common.RequestIdKey), Status: code,
		})
		return
	}
	data := gin.H{
		"task_id": task.TaskID,
		"status":  jimengTaskStatus(task.Status),
	}
	if task.Status == model.TaskStatusSuccess {
		data["video_url"] = task.GetResultURL()
	}
	c.JSON(http.StatusOK, gin.H{
		"code":       10000,
		"message":    "Success",
		"request_id": c.GetString(common.RequestIdKey),
		"status":     10000,
		"data":       data,
	})
}

func videoTaskForProtocol(userID int, taskID, protocol string) (*model.Task, bool, error) {
	return model.GetTaskForProtocol(userID, taskID, protocol, false)
}

func klingTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeed"
	case model.TaskStatusFailure, model.TaskStatusProviderContractFailure, model.TaskStatusCancelled, model.TaskStatusExpired:
		return "failed"
	case model.TaskStatusInProgress:
		return "processing"
	default:
		return "submitted"
	}
}

func jimengTaskStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "done"
	case model.TaskStatusFailure, model.TaskStatusProviderContractFailure, model.TaskStatusCancelled, model.TaskStatusExpired:
		return "failed"
	case model.TaskStatusInProgress:
		return "generating"
	default:
		return "in_queue"
	}
}
