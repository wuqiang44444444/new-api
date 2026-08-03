package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPersistentMediaImageTaskRequestScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newContext := func(channelType int, converter string, modelName string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"`+modelName+`"}`))
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolOpenAIImages)
		common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
		common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
			AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
				IncomingPath: "/v1/images/generations",
				UpstreamPath: "/v1/images/generations",
				Converter:    converter,
				Models:       []string{modelName},
			}}},
		})
		return c
	}

	assert.True(t, isPersistentMediaImageTaskRequest(newContext(
		constant.ChannelTypeAdvancedCustom,
		dto.AdvancedCustomConverterMediaTaskImageBlocking,
		"seedream-5",
	)))
	assert.False(t, isPersistentMediaImageTaskRequest(newContext(
		constant.ChannelTypeOpenAI,
		dto.AdvancedCustomConverterMediaTaskImageBlocking,
		"gpt-image-2",
	)))
	assert.False(t, isPersistentMediaImageTaskRequest(newContext(
		constant.ChannelTypeAdvancedCustom,
		"none",
		"gpt-image-2",
	)))
}

func TestImageTaskCreateIdempotencyDoesNotClaimForGPTImage2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	claimObserved := false
	engine.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolOpenAIImages)
		common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-2")
		c.Next()
	})
	engine.Use(ImageTaskCreateIdempotency())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		_, claimObserved = common.GetContextKey(c, constant.ContextKeyTaskIdempotencyID)
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations",
		strings.NewReader(`{"model":"gpt-image-2","prompt":"a cat"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "gpt-image-2-must-not-claim")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.False(t, claimObserved)
	assert.NotEqual(t, http.StatusConflict, recorder.Code)
	assert.NotEqual(t, http.StatusInternalServerError, recorder.Code)
}
