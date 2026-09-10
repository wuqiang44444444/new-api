package seedance

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// buildMoxingModelArkRequest preserves the northbound pointer semantics instead
// of reserializing through requestPayload's non-pointer strings and slices.
func buildMoxingModelArkRequest(c *gin.Context, payload *requestPayload) ([]byte, error) {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return nil, relaycommon.NewVideoContractError("invalid_video_contract", "Seedance requires the ModelArk V3 request contract")
	}
	body := *contract.ModelArk
	body.Model = payload.Model
	if body.GenerateAudio == nil && payload.GenerateAudio != nil {
		body.GenerateAudio = common.GetPointer(bool(*payload.GenerateAudio))
	}
	// frames already specifies the requested length. Only a request with neither
	// length field uses the published five-second default, never upstream -1.
	if body.Duration == nil && body.Frames == nil {
		model, ok := dto.MoxingVideoModelContractFor(body.Model)
		if !ok {
			return nil, relaycommon.NewVideoContractError("invalid_video_parameter", "the selected customer model is not supported by its configured video adapter")
		}
		body.Duration = common.GetPointer(model.DefaultDurationSeconds)
	}
	return common.Marshal(body)
}
