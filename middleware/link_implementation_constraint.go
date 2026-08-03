package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// LinkImplementationChannelConstraint filters explicitly registered Link SKUs
// before the generic NEWAPI distributor runs. Unregistered/native models retain
// the original distributor behavior.
func LinkImplementationChannelConstraint() gin.HandlerFunc {
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
		if !shouldSelectChannel || request == nil || !model.IsRegisteredLinkSKU(request.Model) {
			c.Next()
			return
		}
		var channels []model.Channel
		if err := model.DB.Where("status = ?", common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Link implementation registry is temporarily unavailable")
			return
		}
		allowed := make(map[int]struct{})
		for i := range channels {
			if model.ValidateChannelLinkImplementationForSKU(&channels[i], request.Model) == nil {
				allowed[channels[i].Id] = struct{}{}
			}
		}
		if existingValue, exists := common.GetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs); exists {
			existing, typeOK := existingValue.(map[int]struct{})
			if !typeOK {
				allowed = nil
			} else {
				for channelID := range allowed {
					if _, exists := existing[channelID]; !exists {
						delete(allowed, channelID)
					}
				}
			}
		}
		if len(allowed) == 0 {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "no explicitly registered Link implementation is available for this SKU")
			return
		}
		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs, allowed)
		c.Next()
	}
}
