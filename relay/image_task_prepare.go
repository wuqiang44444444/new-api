package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

// PreparePersistentImageTaskRequest marks only the protocol-neutral Advanced
// Custom media-task image route for durable async billing. Other image channels
// keep the existing synchronous request lifecycle unchanged.
func PreparePersistentImageTaskRequest(c *gin.Context, info *relaycommon.RelayInfo) {
	if info == nil || info.RelayMode != relayconstant.RelayModeImagesGenerations {
		return
	}
	info.InitChannelMeta(c)
	if info.ChannelType != constant.ChannelTypeAdvancedCustom {
		return
	}
	config := info.ChannelOtherSettings.AdvancedCustom
	if config == nil {
		return
	}
	if !config.SupportsPersistentMediaImageTask("/v1/images/generations", info.OriginModelName) {
		return
	}
	info.ForcePreConsume = true
	common.SetContextKey(c, constant.ContextKeyTaskPersistenceEnabled, true)
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{
		Action:         constant.TaskActionImageGeneration,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
	}
}
