package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// NativeOpenAIVideoContractConstraint keeps the restored rc23 OpenAI-compatible
// routes separate from the explicitly registered Link SKU contracts.
func NativeOpenAIVideoContractConstraint() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		request, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		if !shouldSelectChannel || request == nil {
			c.Next()
			return
		}
		common.SetContextKey(c, constant.ContextKeyOriginalModel, request.Model)
		if model.IsRegisteredLinkSKU(request.Model) {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "registered Link SKUs must use their published Link contract",
				"type":    "invalid_request_error",
				"code":    "link_sku_contract_mismatch",
			}})
			return
		}
		c.Next()
	}
}
