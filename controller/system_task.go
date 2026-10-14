package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func CreateLogCleanupSystemTask(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "target timestamp is required",
		})
		return
	}

	task, err := service.StartLogCleanupTask(targetTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    task.ToResponse(),
	})
}

// CreateEvidenceCleanupSystemTask 是 Root 手动“清理请求证据”入口：
// POST JSON {target_timestamp}。截止时间必须为有效正整数且不晚于当前时间，
// 等于截止点的记录保留；操作者从已鉴权身份取得，不接受客户端伪造。
// 已有清理任务运行时返回明确忙碌及当前任务信息，不修改正在运行的范围。
func CreateEvidenceCleanupSystemTask(c *gin.Context) {
	var request struct {
		TargetTimestamp int64 `json:"target_timestamp"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	now := common.GetTimestamp()
	if request.TargetTimestamp <= 0 || request.TargetTimestamp > now {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "target timestamp must be a positive value not later than now"})
		return
	}
	task, created, err := service.StartEvidenceCleanupTask(request.TargetTimestamp, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "an evidence cleanup task is already active",
			"data":    task.ToResponse(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    task.ToResponse(),
	})
}

func GetCurrentSystemTask(c *gin.Context) {
	taskType := c.Query("type")
	if taskType == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "type is required",
		})
		return
	}

	task, err := model.GetActiveSystemTask(taskType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    task.ToResponse(),
	})
}

func ListSystemTasks(c *gin.Context) {
	var query struct {
		model.SystemTaskFilter
		Offset int `form:"offset" binding:"gte=0"`
		Limit  int `form:"limit"`
	}
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid system task filters"})
		return
	}

	tasks, total, err := model.ListSystemTasks(query.SystemTaskFilter, query.Offset, query.Limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	responses := make([]model.SystemTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, task.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responses,
		"total":   total,
	})
}

func DeleteSystemTaskHistory(c *gin.Context) {
	var filter model.SystemTaskFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid system task filters"})
		return
	}
	deleted, err := model.DeleteSystemTaskHistory(filter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"deleted_count": deleted},
	})
}

func GetSystemTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "task id is required",
		})
		return
	}

	task, err := model.GetSystemTaskByTaskID(taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "task not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    task.ToResponse(),
	})
}
