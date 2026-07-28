package doubao

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
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
	payload := &requestPayload{}
	encoded, err := common.Marshal(contract.ModelArk)
	if err != nil {
		return nil, true, err
	}
	if err := common.Unmarshal(encoded, payload); err != nil {
		return nil, true, err
	}
	return payload, true, nil
}

func (a *TaskAdaptor) validateModelArkContract(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok {
		return nil
	}
	if contract.ContractID != dto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("incompatible Seedance video contract"),
			"invalid_video_contract",
			http.StatusBadRequest,
		)
	}
	if reason := dto.ModelArkVideoProfileIncompatibility(contract.ModelArk, a.profile, info.ChannelOtherSettings.AllowServiceTier); reason != "" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("%s", reason),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}
	return nil
}
