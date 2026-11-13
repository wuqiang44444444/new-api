package seedance

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func (a *TaskAdaptor) modelArkContractPayload(c *gin.Context) (*requestPayload, bool, error) {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok {
		return nil, false, nil
	}
	if contract.ContractID != dto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, true, relaycommon.NewVideoContractError("invalid_video_contract", "Seedance route received an incompatible video contract")
	}
	payload, err := modelArkPayload(contract.ModelArk)
	return payload, true, err
}

func modelArkPayload(request *dto.ModelArkVideoCreateRequest) (*requestPayload, error) {
	payload := &requestPayload{}
	encoded, err := common.Marshal(request)
	if err != nil {
		return nil, err
	}
	if err := common.Unmarshal(encoded, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
