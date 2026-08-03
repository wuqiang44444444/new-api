package doubao

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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
	if subjectHash := common.GetContextKeyString(c, constant.ContextKeyEndUserSubjectHash); subjectHash != "" {
		payload.SafetyIdentifier = subjectHash
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
	if taskErr := service.ValidateFrozenVideoSKUCapability(c, info); taskErr != nil {
		return taskErr
	}
	return nil
}
