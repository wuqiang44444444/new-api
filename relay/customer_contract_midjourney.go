package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func prepareCustomerContractMidjourney(c *gin.Context, info *relaycommon.RelayInfo, publicModel string) error {
	fact, err := service.ResolveCustomerContractLockedChannel(c, publicModel, c.GetInt("channel_id"))
	if err != nil || fact == nil {
		return err
	}
	info.OriginModelName = publicModel
	info.ContractBillingFact = fact
	info.InitChannelMeta(c)
	info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	info.UsingGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	return nil
}
