package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// ImageAsyncExecutionRequested selects the platform task lifecycle after initial
// channel distribution, before idempotency or billing. Channels without a task
// executor keep their native image response even when the client prefers async.
// Keep that choice for the entire request: a native retry must not transfer an
// already pre-consumed request into a second, task-owned billing lifecycle.
func ImageAsyncExecutionRequested(c *gin.Context) bool {
	if !PreferRespondAsync(c) {
		return false
	}
	const contextKey = "image_async_execution"
	if selected, exists := c.Get(contextKey); exists {
		return selected.(bool)
	}
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	async := channelType == constant.ChannelTypeGemini ||
		channelType == constant.ChannelTypeVertexAi ||
		channelType == constant.ChannelTypeAsyncImage
	if channelType == constant.ChannelTypeOpenAI || channelType == constant.ChannelTypeAzure {
		// Task priority: an explicit async preference selects the platform task
		// lifecycle even when the client also asks for stream=true. The frozen
		// request keeps stream, and the background worker consumes the upstream
		// stream before delivering through the task query. This is mode
		// selection, not a second image contract: body-level errors (invalid
		// JSON, invalid stream value, limits) stay with the authoritative
		// native request validator, which runs before admission.
		async = true
	}
	c.Set(contextKey, async)
	return async
}
