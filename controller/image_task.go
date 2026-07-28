package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func OpenAIImageTaskGet(c *gin.Context) {
	taskID := c.Param("task_id")
	var (
		task   *model.Task
		exists bool
		err    error
	)
	if model.IsAdmin(c.GetInt("id")) {
		task, exists, err = model.GetByOnlyTaskId(taskID)
		if exists && (task.ClientProtocol != model.TaskClientProtocolOpenAIImages || task.ClientDeletedAt != 0) {
			exists = false
		}
	} else {
		task, exists, err = model.GetTaskForProtocol(c.GetInt("id"), taskID, model.TaskClientProtocolOpenAIImages, false)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": dto.ImageTaskError{
			Code: "internal_error", Message: "Image task could not be loaded",
		}})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": dto.ImageTaskError{
			Code: "task_not_found", Message: "Image task was not found",
		}})
		return
	}
	c.JSON(http.StatusOK, model.ProjectOpenAIImageTask(task))
}
