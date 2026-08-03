package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func attachTaskProtocolSnapshot(c *gin.Context, task *model.Task, info *relaycommon.RelayInfo) {
	if task == nil || info == nil || info.TaskRelayInfo == nil {
		return
	}
	task.ClientProtocol = info.TaskRelayInfo.ClientProtocol
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok {
		task.PrivateData.NorthboundContractID = string(contract.ContractID)
		switch contract.ContractID {
		case dto.VideoContractModelArkV3:
			task.PrivateData.NorthboundContractVersion = "2024-01-01"
		case dto.VideoContractKlingV1:
			task.PrivateData.NorthboundContractVersion = "v1"
		case dto.VideoContractJimeng:
			task.PrivateData.NorthboundContractVersion = "2022-08-31"
		}
		task.PrivateData.SouthboundAdapterVersion = relaycommon.CurrentVideoSouthboundAdapterVersion(
			info.ChannelType,
			info.ChannelOtherSettings.VideoUpstreamProfile,
		)
	}
	if capability, ok := common.GetContextKeyType[model.VideoSKUCapability](c, constant.ContextKeyResolvedVideoSKUCapability); ok {
		task.PrivateData.SKUCapabilityVersion = capability.Version
		task.PrivateData.SKUCapabilityHash = capability.ContentHash
		task.PrivateData.SKULifecycle = capability.Lifecycle
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return
	}
	seconds := req.Seconds
	if seconds == "" && req.Duration > 0 {
		seconds = strconv.Itoa(req.Duration)
	}
	serviceTier, _ := req.Metadata["service_tier"].(string)
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok && contract.ModelArk != nil && contract.ModelArk.ServiceTier != nil {
		serviceTier = *contract.ModelArk.ServiceTier
	}
	task.PrivateData.ClientRequest = model.TaskClientRequestSnapshot{
		Prompt:             req.Prompt,
		Seconds:            seconds,
		Size:               req.Size,
		RemixedFromVideoID: info.OriginTaskID,
		ServiceTier:        serviceTier,
	}
}
