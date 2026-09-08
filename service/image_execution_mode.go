package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
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
		// This is mode selection, not a second image contract. Read through the
		// shared storage before idempotency can claim the request. Invalid input
		// stays native so the authoritative request validator reports the error.
		var stream bool
		var err error
		if strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			form, parseErr := common.ParseMultipartFormReusable(c)
			err = parseErr
			if err == nil {
				defer form.RemoveAll()
				if values := form.Value["stream"]; len(values) > 0 && strings.TrimSpace(values[0]) != "" {
					stream, err = strconv.ParseBool(strings.TrimSpace(values[0]))
				}
			}
		} else {
			var mode struct {
				Stream *bool `json:"stream"`
			}
			err = common.UnmarshalBodyReusable(c, &mode)
			stream = mode.Stream != nil && *mode.Stream
		}
		async = err == nil && !stream
	}
	c.Set(contextKey, async)
	return async
}
