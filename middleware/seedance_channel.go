package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
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
		// The contract provides the discount only. It never picks or constrains
		// the Seedance channel or group: a model without a contract discount
		// keeps native behavior, and resolution anomalies fail closed.
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
		// 已迁移协议在进入计价/预扣前固定 seedance-link 插件版本；插件不可
		// 用时失败关闭，不产生任何资金动作。未迁移协议不打 pin，继续 Go 路径。
		if pinErr := taskseedance.PinSeedanceExtensionForChannel(c, channel.GetOtherSettings().VideoUpstreamProtocol); pinErr != nil {
			abortModelArkVideo(c, http.StatusServiceUnavailable, "seedance_plugin_unavailable", "Seedance plugin extension is unavailable")
			return
		}
		common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
		c.Next()
	}
}
