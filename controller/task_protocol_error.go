package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func respondTaskProtocolError(c *gin.Context, taskErr *dto.TaskError) bool {
	if c == nil || taskErr == nil {
		return false
	}
	protocol := common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol)
	if protocol != model.TaskClientProtocolOpenAIVideos && protocol != model.TaskClientProtocolModelArkV3 {
		return false
	}
	status, code, errorType, message := taskProtocolErrorFields(taskErr)
	if protocol == model.TaskClientProtocolModelArkV3 {
		modelArkVideoError(c, status, code, message)
		return true
	}
	openAIVideoError(c, status, errorType, code, message)
	return true
}

func taskProtocolErrorFields(taskErr *dto.TaskError) (status int, code, errorType, message string) {
	status = taskErr.StatusCode
	code = strings.TrimSpace(taskErr.Code)
	if code == "" {
		code = "task_request_failed"
	}
	switch {
	case !taskErr.LocalError && (status == http.StatusUnauthorized || status == http.StatusForbidden):
		status = http.StatusBadGateway
		errorType = "server_error"
		code = "upstream_auth_error"
		message = "Video service credentials are unavailable"
	case status == http.StatusTooManyRequests:
		errorType = "rate_limit_error"
		code = "rate_limit_exceeded"
		message = "Video service is busy; retry later"
	case status == http.StatusUnauthorized:
		errorType = "authentication_error"
		code = "authentication_error"
		message = "Authentication failed"
	case status == http.StatusForbidden:
		errorType = "permission_error"
		code = "permission_denied"
		message = "Permission denied"
	case status == http.StatusPaymentRequired:
		errorType = "insufficient_quota"
		code = "insufficient_quota"
		message = "Insufficient quota"
	case status == http.StatusNotFound:
		errorType = "invalid_request_error"
		code = "not_found"
		message = "Video resource was not found"
	case status == http.StatusConflict:
		errorType = "invalid_request_error"
		code = "conflict"
		message = "Video request conflicts with the current resource state"
	case status >= http.StatusInternalServerError:
		errorType = "server_error"
		if taskErr.LocalError {
			code = "internal_error"
			message = "Video service encountered an internal error"
			break
		}
		code = "upstream_unavailable"
		message = "Video service is temporarily unavailable"
	default:
		errorType = "invalid_request_error"
		message = strings.TrimSpace(taskErr.Message)
		if message == "" {
			message = "Invalid video request"
		}
	}
	return status, code, errorType, message
}
