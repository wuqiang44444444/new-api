package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func setTaskCreateContractResponse(c *gin.Context, task *model.Task) {
	if c == nil || task == nil {
		return
	}
	switch task.ClientProtocol {
	case model.TaskClientProtocolModelArkV3:
		c.Set(middleware.TaskCreateContractResponseKey, gin.H{"id": task.TaskID})
	case model.TaskClientProtocolKlingV1:
		c.Set(middleware.TaskCreateContractResponseKey, gin.H{
			"code":       0,
			"message":    "SUCCEED",
			"request_id": c.GetString(common.RequestIdKey),
			"data": gin.H{
				"task_id":     task.TaskID,
				"task_status": "submitted",
			},
		})
	case model.TaskClientProtocolJimeng:
		c.Set(middleware.TaskCreateContractResponseKey, gin.H{
			"code":       10000,
			"message":    "Success",
			"request_id": c.GetString(common.RequestIdKey),
			"status":     10000,
			"data":       gin.H{"task_id": task.TaskID},
		})
	}
}

func setTaskCreateContractPersistenceError(c *gin.Context, protocol string) {
	requestID := c.GetString(common.RequestIdKey)
	// 错误事件诊断（方案 §3.3）：unknown 合同出口补齐渠道、模型、阶段与稳定原因，
	// 复用现有错误事件，不新增诊断表，不向客户暴露 Provider 私有信息。
	if c.Request != nil {
		clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
			Stage:     "task_create_persist",
			Reason:    "create_outcome_unknown",
			Model:     clienterrlog.SanitizeLogValue(c.GetString("original_model"), 128),
			ChannelID: common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			Protocol:  clienterrlog.SanitizeLogValue(protocol, 64),
		})
	}
	var body any = gin.H{"error": gin.H{
		"message":    common.MessageWithRequestId("Task creation outcome is unknown. Do not automatically retry; held funds are released after 24 hours", requestID),
		"type":       "server_error",
		"code":       "create_outcome_unknown",
		"request_id": requestID,
	}}
	if protocol == model.TaskClientProtocolModelArkV3 {
		body = gin.H{"error": gin.H{
			"code":       "create_outcome_unknown",
			"message":    common.MessageWithRequestId("Task creation outcome is unknown. Do not automatically retry; held funds are released after 24 hours", requestID),
			"request_id": requestID,
		}}
	}
	if protocol == model.TaskClientProtocolKlingV1 {
		body = dto.KlingVideoErrorResponse{
			Code:      dto.KlingVideoErrorCode(http.StatusServiceUnavailable),
			Message:   common.MessageWithRequestId("Task creation outcome is unknown. Do not automatically retry; held funds are released after 24 hours", requestID),
			RequestID: requestID,
			Data:      nil,
		}
	}
	if protocol == model.TaskClientProtocolJimeng {
		code := dto.JimengVideoErrorCode(http.StatusServiceUnavailable)
		body = dto.JimengVideoErrorResponse{
			Code:      code,
			Data:      nil,
			Message:   common.MessageWithRequestId("Task creation outcome is unknown. Do not automatically retry; held funds are released after 24 hours", requestID),
			RequestID: requestID,
			Status:    code,
		}
	}
	c.Set(middleware.TaskCreateContractErrorKey, middleware.TaskCreateContractError{
		Status: http.StatusServiceUnavailable,
		Body:   body,
	})
}
