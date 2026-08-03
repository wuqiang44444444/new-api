package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func abortOfficialVideo(c *gin.Context, status int, message string) bool {
	switch common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol) {
	case model.TaskClientProtocolKlingV1:
		abortKlingVideo(c, status, message)
		return true
	case model.TaskClientProtocolJimeng:
		abortJimengVideo(c, status, message)
		return true
	default:
		return false
	}
}

func abortKlingVideo(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, dto.KlingVideoErrorResponse{
		Code:      dto.KlingVideoErrorCode(status),
		Message:   message,
		RequestID: c.GetString(common.RequestIdKey),
		Data:      nil,
	})
}
