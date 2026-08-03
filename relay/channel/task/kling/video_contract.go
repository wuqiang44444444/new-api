package kling

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func klingContractPayload(c *gin.Context, info *relaycommon.RelayInfo) (*requestPayload, bool, error) {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok {
		return nil, false, nil
	}
	if contract.ContractID != dto.VideoContractKlingV1 || contract.Kling == nil {
		return nil, true, relaycommon.NewVideoContractError("invalid_video_contract", "Kling route received an incompatible video contract")
	}
	payload := &requestPayload{
		Mode:         "std",
		Duration:     "5",
		AspectRatio:  "1:1",
		ModelName:    info.UpstreamModelName,
		Model:        info.UpstreamModelName,
		CfgScale:     0.5,
		DynamicMasks: []DynamicMask{},
	}
	encoded, err := common.Marshal(contract.Kling)
	if err != nil {
		return nil, true, err
	}
	if err := common.Unmarshal(encoded, payload); err != nil {
		return nil, true, err
	}
	if strings.TrimSpace(info.UpstreamModelName) != "" {
		payload.ModelName = info.UpstreamModelName
		payload.Model = info.UpstreamModelName
	}
	if payload.ModelName == "" {
		payload.ModelName = "kling-v1"
		payload.Model = "kling-v1"
	}
	return payload, true, nil
}
