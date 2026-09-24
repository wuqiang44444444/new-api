package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskminimax "github.com/QuantumNous/new-api/relay/channel/task/minimax"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// ResolveStandardVideoChannel routes the shared ModelArk V3 standard entry
// across the code-registered typed video channels. A customer model enabled
// on a MiniMax Link channel resolves through the MiniMax typed path; every
// other request keeps the exact Seedance resolution behavior. Identity comes
// only from the enabled channel list — never from model names, domains,
// prices, keys or request fields.
func ResolveStandardVideoChannel() gin.HandlerFunc {
	seedanceResolve := ResolveSeedanceChannel()
	minimaxResolve := resolveMiniMaxLinkChannel()
	return func(c *gin.Context) {
		if servesMiniMaxLinkChannel(c) {
			minimaxResolve(c)
			return
		}
		seedanceResolve(c)
	}
}

// servesMiniMaxLinkChannel performs the cheap management-approved probe: an
// enabled MiniMax Link channel whose model list contains the contract model.
// Save/enable validation guarantees cross-type uniqueness, so this probe
// cannot disagree with the Seedance lookup for an enabled model.
func servesMiniMaxLinkChannel(c *gin.Context) bool {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return false
	}
	customerModel := strings.TrimSpace(contract.ModelArk.Model)
	if customerModel == "" {
		return false
	}
	return model.MiniMaxLinkChannelServesModel(customerModel)
}

// resolveMiniMaxLinkChannel mirrors the typed resolution flow of the Seedance
// middleware for the MiniMax Link channel type: contract authorization,
// token model limit, deterministic channel selection, channel context setup
// and the fail-closed extension pin before any pricing or funds movement.
func resolveMiniMaxLinkChannel() gin.HandlerFunc {
	return func(c *gin.Context) {
		contract, ok := relaycommon.GetVideoContractRequest(c)
		if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
			abortModelArkVideo(c, http.StatusBadRequest, "invalid_video_contract", "MiniMax request contract is unavailable")
			return
		}
		customerModel := strings.TrimSpace(contract.ModelArk.Model)
		if _, err := applyCustomerContractRequest(c, customerModel); err != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "Contract authorization is unavailable")
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
		if pin, found, _ := service.GetChannelConstraints(c).ResolvedPin(); found {
			specificChannelID = pin.ChannelId
		}

		usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
		groups := []string{usingGroup}
		if usingGroup == "auto" {
			groups = service.GetRequestAutoGroups(c, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
		}
		var selectedGroup string
		var channel *model.Channel
		if service.ActiveCustomerContract(c) != nil {
			var err error
			channel, selectedGroup, err = service.CustomerContractTypedChannel(c, customerModel, constant.ChannelTypeMiniMaxLink, specificChannelID)
			if err != nil {
				abortModelArkVideo(c, http.StatusServiceUnavailable, "model_not_found", "合同范围内无可用模型或渠道 / Contract model or channel is unavailable")
				return
			}
			groups = nil
		}
		for _, group := range groups {
			candidate, err := model.GetEnabledMiniMaxChannel(group, customerModel, specificChannelID)
			if err != nil {
				abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "MiniMax channel lookup failed")
				return
			}
			if candidate != nil {
				channel = candidate
				selectedGroup = group
				break
			}
		}
		if channel == nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "model_not_found", "no enabled MiniMax channel is configured for this model")
			return
		}
		if usingGroup == "auto" {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, selectedGroup)
		}
		if setupErr := SetupContextForSelectedChannel(c, channel, customerModel); setupErr != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "upstream_unavailable", "MiniMax channel is unavailable")
			return
		}
		// 在进入计价/预扣前固定 minimax-link 插件版本；插件不可用时失败关闭，
		// 不产生任何资金动作。
		if pinErr := taskminimax.PinMinimaxExtensionForChannel(c); pinErr != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "minimax_plugin_unavailable", "MiniMax plugin extension is unavailable")
			return
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		c.Next()
	}
}
