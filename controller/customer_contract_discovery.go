package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func respondRestrictedModelList(c *gin.Context, modelType int) bool {
	if !middleware.APIKeyModelAccessRestricted(c) {
		return false
	}
	switch modelType {
	case constant.ChannelTypeAnthropic:
		c.JSON(http.StatusOK, gin.H{"data": []dto.AnthropicModel{}, "first_id": "", "last_id": "", "has_more": false})
	case constant.ChannelTypeGemini:
		c.JSON(http.StatusOK, gin.H{"models": []dto.GeminiModel{}, "nextPageToken": nil})
	default:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []dto.OpenAIModels{}, "object": "list"})
	}
	return true
}

func respondRestrictedModelDetail(c *gin.Context) bool {
	if !middleware.APIKeyModelAccessRestricted(c) {
		return false
	}
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
		"type": "invalid_request_error", "code": "model_not_found", "message": i18n.T(c, i18n.MsgAPIKeyContractRequired),
	}})
	return true
}
