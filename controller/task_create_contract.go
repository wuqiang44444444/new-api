package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func setTaskCreateContractResponse(c *gin.Context, task *model.Task) {
	if c == nil || task == nil {
		return
	}
	switch task.ClientProtocol {
	case model.TaskClientProtocolOpenAIVideos:
		c.Set(middleware.TaskCreateContractResponseKey, task.ToOpenAIVideo())
	case model.TaskClientProtocolModelArkV3:
		c.Set(middleware.TaskCreateContractResponseKey, gin.H{"id": task.TaskID})
	}
}

func setTaskCreateContractPersistenceError(c *gin.Context, protocol string) {
	requestID := c.GetString(common.RequestIdKey)
	body := gin.H{"error": gin.H{
		"message":    common.MessageWithRequestId("Task creation outcome requires reconciliation", requestID),
		"type":       "server_error",
		"code":       "create_outcome_unknown",
		"request_id": requestID,
	}}
	if protocol == model.TaskClientProtocolModelArkV3 {
		body = gin.H{"error": gin.H{
			"code":       "create_outcome_unknown",
			"message":    common.MessageWithRequestId("Task creation outcome requires reconciliation", requestID),
			"request_id": requestID,
		}}
	}
	c.Set(middleware.TaskCreateContractErrorKey, middleware.TaskCreateContractError{
		Status: http.StatusServiceUnavailable,
		Body:   body,
	})
}
