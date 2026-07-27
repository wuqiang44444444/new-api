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
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ModelArkVideoList(c *gin.Context) {
	pageNum, ok := modelArkPageValue(c, "page_num", 1)
	if !ok || pageNum > 500 {
		modelArkVideoError(c, http.StatusBadRequest, "invalid_page_num", "page_num must be between 1 and 500")
		return
	}
	pageSize, ok := modelArkPageValue(c, "page_size", 10)
	if !ok || pageSize > 500 {
		modelArkVideoError(c, http.StatusBadRequest, "invalid_page_size", "page_size must be between 1 and 500")
		return
	}
	taskIDs := c.QueryArray("filter.task_ids")
	if len(taskIDs) > 500 {
		modelArkVideoError(c, http.StatusBadRequest, "invalid_filter", "filter.task_ids supports at most 500 values")
		return
	}
	statuses, valid := model.ModelArkTaskStatuses(c.Query("filter.status"))
	if !valid {
		modelArkVideoError(c, http.StatusBadRequest, "invalid_status", "filter.status is invalid")
		return
	}
	serviceTier := strings.TrimSpace(c.Query("filter.service_tier"))
	if serviceTier == "" {
		serviceTier = "default"
	}
	tasks, total, err := model.ListModelArkVideoTasks(c.GetInt("id"), model.ModelArkTaskListFilter{
		Statuses:    statuses,
		TaskIDs:     taskIDs,
		Model:       c.Query("filter.model"),
		ServiceTier: serviceTier,
		PageNum:     pageNum,
		PageSize:    pageSize,
		Now:         common.GetTimestamp(),
	})
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to list tasks")
		return
	}
	response := &dto.ModelArkVideoTaskList{Total: total, Items: make([]*dto.ModelArkVideoTask, 0, len(tasks))}
	for i := range tasks {
		response.Items = append(response.Items, projectModelArkVideoTask(c, &tasks[i]))
	}
	c.JSON(http.StatusOK, response)
}

func ModelArkVideoGet(c *gin.Context) {
	task, exists, err := model.GetVideoTaskForProtocol(c.GetInt("id"), c.Param("task_id"), model.TaskClientProtocolModelArkV3, false)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to retrieve task")
		return
	}
	if !exists {
		modelArkVideoError(c, http.StatusNotFound, "task_not_found", "task not found")
		return
	}
	c.JSON(http.StatusOK, projectModelArkVideoTask(c, task))
}

func ModelArkVideoDelete(c *gin.Context) {
	task, exists, err := model.GetVideoTaskForProtocol(c.GetInt("id"), c.Param("task_id"), model.TaskClientProtocolModelArkV3, false)
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to delete task")
		return
	}
	if !exists {
		modelArkVideoError(c, http.StatusNotFound, "task_not_found", "task not found")
		return
	}
	switch task.Status {
	case model.TaskStatusNotStart, model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusUnknown:
		modelArkCancelQueuedTask(c, task)
	case model.TaskStatusInProgress:
		modelArkVideoError(c, http.StatusConflict, "task_running", "a running task cannot be deleted")
	case model.TaskStatusCancelled:
		modelArkVideoError(c, http.StatusConflict, "task_cancelled", "a cancelled task cannot be deleted again")
	case model.TaskStatusSuccess, model.TaskStatusFailure, model.TaskStatusExpired:
		modelArkDeleteTerminalTask(c, task)
	default:
		modelArkVideoError(c, http.StatusConflict, "invalid_task_state", "task state does not allow deletion")
	}
}

func modelArkCancelQueuedTask(c *gin.Context, current *model.Task) {
	begin, err := model.BeginTaskCancellation(c.GetInt("id"), current.TaskID, model.TaskClientProtocolModelArkV3)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		modelArkVideoError(c, http.StatusNotFound, "task_not_found", "task not found")
		return
	}
	if err != nil || begin == nil || begin.Task == nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to start task cancellation")
		return
	}
	if !begin.ShouldCall {
		modelArkVideoError(c, http.StatusConflict, "cancellation_in_progress", "task cancellation is already in progress")
		return
	}
	providerChannel, err := videoTaskProviderChannel(begin.Task)
	if err != nil {
		_, _, _ = model.CompleteTaskCancellation(begin.Task.ID, false, false, "provider channel unavailable")
		modelArkVideoError(c, http.StatusServiceUnavailable, "upstream_unavailable", "video service is temporarily unavailable")
		return
	}
	err = relay.CancelQueuedVideoTask(c.Request.Context(), begin.Task, providerChannel)
	if err != nil {
		rejected := channel.IsDefinitiveTaskLifecycleRejection(err)
		_, _, _ = model.CompleteTaskCancellation(begin.Task.ID, false, rejected, "provider cancellation did not complete")
		if rejected {
			modelArkVideoError(c, http.StatusConflict, "cancellation_rejected", "the task can no longer be cancelled")
		} else {
			modelArkVideoError(c, http.StatusServiceUnavailable, "cancellation_unknown", "cancellation result is unknown; retry task retrieval")
		}
		return
	}
	cancelled, wonTerminal, err := model.CompleteTaskCancellation(begin.Task.ID, true, false, "")
	if err != nil {
		modelArkVideoError(c, http.StatusInternalServerError, "internal_error", "failed to complete task cancellation")
		return
	}
	if wonTerminal {
		service.SettleConfirmedTaskCancellation(c.Request.Context(), cancelled)
	} else if cancelled.Status != model.TaskStatusCancelled {
		modelArkVideoError(c, http.StatusConflict, "cancellation_lost_race", "the task reached a terminal state before cancellation completed")
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func modelArkDeleteTerminalTask(c *gin.Context, task *model.Task) {
	providerChannel, err := videoTaskProviderChannel(task)
	if err != nil {
		modelArkVideoError(c, http.StatusServiceUnavailable, "upstream_unavailable", "video service is temporarily unavailable")
		return
	}
	if err := relay.DeleteTerminalVideoTask(c.Request.Context(), task, providerChannel); err != nil {
		if channel.IsDefinitiveTaskLifecycleRejection(err) {
			modelArkVideoError(c, http.StatusConflict, "delete_rejected", "the task cannot be deleted in its current state")
		} else {
			modelArkVideoError(c, http.StatusServiceUnavailable, "upstream_unavailable", "task deletion did not complete")
		}
		return
	}
	deleted, err := model.MarkVideoTaskClientDeleted(c.GetInt("id"), task.TaskID, model.TaskClientProtocolModelArkV3)
	if err != nil || !deleted {
		modelArkVideoError(c, http.StatusConflict, "delete_conflict", "task deletion could not be completed")
		return
	}
	c.JSON(http.StatusOK, gin.H{})
}

func modelArkPageValue(c *gin.Context, name string, fallback int) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil && value >= 1
}

func modelArkVideoError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"code":       code,
		"message":    common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
		"request_id": c.GetString(common.RequestIdKey),
	}})
}
