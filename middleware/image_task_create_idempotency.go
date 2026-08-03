package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ImageTaskCreateIdempotency applies task idempotency only to the selected
// Advanced Custom media-image route. It must run after Distribute so ordinary
// OpenAI image models never acquire a task idempotency claim.
func ImageTaskCreateIdempotency() gin.HandlerFunc {
	taskIdempotency := TaskCreateIdempotency()
	return func(c *gin.Context) {
		if !isPersistentMediaImageTaskRequest(c) {
			c.Next()
			return
		}
		taskIdempotency(c)
	}
}

func isPersistentMediaImageTaskRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return false
	}
	if common.GetContextKeyString(c, constant.ContextKeyTaskClientProtocol) != model.TaskClientProtocolOpenAIImages {
		return false
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeAdvancedCustom {
		return false
	}
	settings, ok := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	if !ok || settings.AdvancedCustom == nil {
		return false
	}
	return settings.AdvancedCustom.SupportsPersistentMediaImageTask(
		c.Request.URL.Path,
		common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
	)
}
