package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// ResolveSeedanceChannel selects the single Channel approved at management
// save/enable time. It deliberately does not call Distribute and therefore does
// not use Priority, Weight, affinity, retry, or fallback.
func ResolveSeedanceChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		contract, ok := relaycommon.GetVideoContractRequest(c)
		if !ok || contract.ContractID != dto.VideoContractModelArkV3 || contract.ModelArk == nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_video_contract", "Seedance request contract is unavailable")
			return
		}
		customerModel := strings.TrimSpace(contract.ModelArk.Model)
		contractFact, err := applyCustomerContractRequest(c, customerModel)
		if err != nil {
			abortModelArkVideo(c, http.StatusForbidden, "model_not_found", "the requested model does not exist")
			return
		}
		if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) {
			value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			limits, valid := value.(map[string]bool)
			if !exists || !valid || !limits[ratio_setting.FormatMatchingModelName(customerModel)] {
				abortModelArkVideo(c, http.StatusForbidden, "model_not_allowed", "this token has no access to the requested model")
				return
			}
		}

		specificChannelID := 0
		if value, exists := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); exists {
			text, valid := value.(string)
			parsed, err := strconv.Atoi(strings.TrimSpace(text))
			if !valid || err != nil {
				abortModelArkVideo(c, http.StatusBadRequest, "invalid_channel", "the token channel is invalid")
				return
			}
			specificChannelID = parsed
		}

		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		if contractFact != nil {
			usingGroup = contractFact.RouteGroup
		}
		groups := []string{usingGroup}
		if usingGroup == "auto" {
			groups = service.GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
		}
		var selectedGroup string
		var channel *model.Channel
		for _, group := range groups {
			candidate, err := model.GetEnabledSeedanceChannel(group, customerModel, specificChannelID)
			if err != nil {
				abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "Seedance channel lookup failed")
				return
			}
			if candidate != nil {
				channel = candidate
				selectedGroup = group
				break
			}
		}
		if channel == nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "model_not_found", "no enabled Seedance channel is configured for this model")
			return
		}
		if usingGroup == "auto" {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, selectedGroup)
		}
		if setupErr := SetupContextForSelectedChannel(c, channel, customerModel); setupErr != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "Seedance channel is unavailable")
			return
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		c.Next()
	}
}
