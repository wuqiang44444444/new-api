package service

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// ValidateFrozenVideoSKUCapability is the adapter-side replay of the same
// versioned capability used by publication and Link contract middleware.
func ValidateFrozenVideoSKUCapability(
	c *gin.Context,
	info *relaycommon.RelayInfo,
) *taskdto.TaskError {
	if c == nil || info == nil || info.ChannelMeta == nil {
		return TaskErrorWrapperLocal(
			fmt.Errorf("video SKU capability context is incomplete"),
			"invalid_video_contract",
			http.StatusBadRequest,
		)
	}
	contract, hasContract := relaycommon.GetVideoContractRequest(c)
	if !hasContract {
		// Legacy routes outside the published Link contract keep their existing adapter
		// validation. Published video routes always carry a typed contract.
		return nil
	}
	capability, ok := common.GetContextKeyType[model.VideoSKUCapability](
		c,
		constant.ContextKeyResolvedVideoSKUCapability,
	)
	if !ok {
		return TaskErrorWrapperLocal(
			fmt.Errorf("video SKU capability snapshot is unavailable"),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}
	channel := &model.Channel{
		Id:     info.ChannelId,
		Type:   info.ChannelType,
		Status: common.ChannelStatusEnabled,
		Models: capability.PublicModel,
	}
	if mapping := common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping); mapping != "" {
		channel.ModelMapping = &mapping
	}
	channel.SetOtherSettings(info.ChannelOtherSettings)
	if err := model.ValidateVideoSKUImplementation(capability, channel); err != nil {
		return TaskErrorWrapperLocal(err, "unsupported_parameter", http.StatusBadRequest)
	}
	if err := capability.ValidateContractRequest(contract); err != nil {
		return TaskErrorWrapperLocal(err, "unsupported_parameter", http.StatusBadRequest)
	}
	return nil
}
