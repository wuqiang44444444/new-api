package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func respondTaskProtocolError(c *gin.Context, taskErr *dto.TaskError) bool {
	if c == nil || taskErr == nil {
		return false
	}
	protocol := common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol)
	if protocol != model.TaskClientProtocolModelArkV3 &&
		protocol != model.TaskClientProtocolKlingV1 &&
		protocol != model.TaskClientProtocolJimeng {
		return false
	}
	info, _ := common.GetContextKeyType[*relaycommon.RelayInfo](c, constant.ContextKeyTaskErrorRelayInfo)
	status, code, _, message := taskProtocolErrorFields(taskErr, info)
	if protocol == model.TaskClientProtocolModelArkV3 {
		modelArkVideoError(c, status, code, message)
		return true
	}
	if protocol == model.TaskClientProtocolKlingV1 {
		c.JSON(status, dto.KlingVideoErrorResponse{
			Code:      dto.KlingVideoErrorCode(status),
			Message:   message,
			RequestID: c.GetString(common.RequestIdKey),
			Data:      nil,
		})
		return true
	}
	if protocol == model.TaskClientProtocolJimeng {
		jimengCode := dto.JimengVideoErrorCode(status)
		c.JSON(status, dto.JimengVideoErrorResponse{
			Code:      jimengCode,
			Data:      nil,
			Message:   message,
			RequestID: c.GetString(common.RequestIdKey),
			Status:    jimengCode,
		})
		return true
	}
	return false
}

func taskProtocolErrorFields(taskErr *dto.TaskError, info *relaycommon.RelayInfo) (status int, code, errorType, message string) {
	originModel, upstreamModel := "", ""
	if info != nil {
		originModel, upstreamModel = info.OriginModelName, info.UpstreamModelName
	}
	status = taskErr.StatusCode
	code = strings.TrimSpace(taskErr.Code)
	message = taskErr.Message
	if taskErr.LocalError && status == http.StatusServiceUnavailable && code == "reference_audio_unavailable" {
		return status, code, "server_error", "Reference audio storage is temporarily unavailable"
	}
	// 已登记安全投影的证据错误：由唯一分类映射输出证据故障类别；未知
	// 本地 5xx 仍走下方通用文案。
	if rejection, ok := service.ClassifyTaskRequestEvidenceRejection(taskErr.Error); ok {
		if rejection.Status >= http.StatusInternalServerError {
			return rejection.Status, rejection.Code, "server_error", rejection.Message
		}
		return rejection.Status, rejection.Code, "invalid_request_error", rejection.Message
	}
	if common.PublicTaskErrorCode(code) == "" && code != "" {
		// Internal operation codes must not expose their diagnostic message.
		message = ""
	}
	code = common.PublicTaskErrorCode(code)
	switch {
	case taskErr.LocalError && status == http.StatusForbidden && code == string(relaykittypes.ErrorCodeInsufficientUserQuota):
		errorType, code, message = "insufficient_quota", "insufficient_quota", "Insufficient quota"
	case !taskErr.LocalError && status == http.StatusUnauthorized:
		status = http.StatusBadGateway
		errorType = "server_error"
		code = "upstream_auth_error"
		message = "Video service credentials are unavailable"
	case !taskErr.LocalError && status == http.StatusForbidden:
		errorType = "invalid_request_error"
		if code == "" {
			code = "upstream_rejected"
		}
		message = common.PublicTaskErrorMessageForModel(message, originModel, upstreamModel)
		if message == "" {
			message = "Video service rejected the request"
		}
	case !taskErr.LocalError && status == http.StatusPaymentRequired:
		status, code, errorType, message = http.StatusBadGateway, "upstream_unavailable", "server_error", "Video service is temporarily unavailable"
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
	case status == http.StatusNotFound || (taskErr.LocalError && code == "task_not_exist"):
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
		if code == "" {
			code = "upstream_unavailable"
		}
		message = common.PublicTaskErrorMessageForModel(message, originModel, upstreamModel)
		if message == "" {
			message = "Video service is temporarily unavailable"
		}
	default:
		errorType = "invalid_request_error"
		if code == "" {
			code = "invalid_request"
		}
		message = common.PublicTaskErrorMessageForModel(message, originModel, upstreamModel)
		if message == "" {
			message = "Invalid video request"
		}
	}
	return status, code, errorType, message
}
