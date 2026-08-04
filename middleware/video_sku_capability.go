package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func ResolveVideoSKUCapability() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		contract, ok := relaycommon.GetVideoContractRequest(c)
		if !ok {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video request contract is unavailable")
			return
		}
		customerModel, modelOK := relaycommon.VideoContractModel(c)
		if !modelOK || strings.TrimSpace(customerModel) == "" {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video model is required")
			return
		}
		publication, published := resolvedLinkModelPublication(c)
		if !published && common.GetContextKeyString(c, constant.ContextKeyLinkRouteFamily) != "" {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video customer model has no published Link contract")
			return
		}
		contractSKU := customerModel
		if published {
			contractSKU = publication.LinkSKU
		}
		capability, registered := model.ResolveVideoSKUCapability(contractSKU)
		if !registered {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video model has no published SKU capability")
			return
		}
		contract = videoContractForPublishedSKU(contract, contractSKU)
		if err := capability.ValidateContractRequest(contract); err != nil {
			abortVideoSKUCapability(c, http.StatusBadRequest, err.Error())
			return
		}
		if common.GetContextKeyString(c, constant.ContextKeyEndUserSubjectHash) != "" && !slices.Contains(capability.RequestFields, "end_user_subject") {
			abortVideoSKUCapability(c, http.StatusBadRequest, "end_user_subject is not supported by this model")
			return
		}
		common.SetContextKey(c, constant.ContextKeyResolvedVideoSKUCapability, capability)
		c.Next()
	}
}

func resolvedVideoSKUCapability(c *gin.Context) (model.VideoSKUCapability, bool) {
	return common.GetContextKeyType[model.VideoSKUCapability](c, constant.ContextKeyResolvedVideoSKUCapability)
}

func VideoSKUChannelConstraint() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		capability, ok := resolvedVideoSKUCapability(c)
		if !ok {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video SKU capability snapshot is unavailable")
			return
		}
		var channels []model.Channel
		if err := model.DB.
			Where("type IN ? AND status = ?", capability.RequiredChannelTypes, common.ChannelStatusEnabled).
			Find(&channels).Error; err != nil {
			abortVideoSKUCapability(c, http.StatusServiceUnavailable, "video service is temporarily unavailable")
			return
		}
		allowed := make(map[int]struct{})
		publication, published := resolvedLinkModelPublication(c)
		if !published && common.GetContextKeyString(c, constant.ContextKeyLinkRouteFamily) != "" {
			abortVideoSKUCapability(c, http.StatusBadRequest, "video publication snapshot is unavailable")
			return
		}
		for i := range channels {
			executionValid := true
			if published {
				executionValid = model.ValidateChannelLinkExecution(&channels[i], publication.CustomerModel, publication.RouteFamily, publication.LinkSKU) == nil
			}
			if model.ValidateVideoSKUImplementation(capability, &channels[i]) == nil && executionValid {
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
			abortVideoSKUCapability(c, http.StatusServiceUnavailable, "no capability-equivalent video channel is available")
			return
		}
		common.SetContextKey(c, constant.ContextKeyAssetAllowedChannelIDs, allowed)
		c.Next()
	}
}

func videoContractForPublishedSKU(contract dto.VideoContractRequest, linkSKU string) dto.VideoContractRequest {
	switch {
	case contract.ModelArk != nil:
		request := *contract.ModelArk
		request.Model = linkSKU
		contract.ModelArk = &request
	case contract.Kling != nil:
		request := *contract.Kling
		request.ModelName = common.GetPointer(linkSKU)
		contract.Kling = &request
	case contract.Jimeng != nil:
		request := *contract.Jimeng
		request.ReqKey = linkSKU
		contract.Jimeng = &request
	}
	return contract
}

func abortVideoSKUCapability(c *gin.Context, status int, message string) {
	contract, _ := relaycommon.GetVideoContractRequest(c)
	if contract.ContractID == dto.VideoContractModelArkV3 {
		abortModelArkVideo(c, status, "unsupported_parameter", message)
		return
	}
	if abortOfficialVideo(c, status, message) {
		return
	}
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
