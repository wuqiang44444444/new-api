package seedance

import (
	"errors"
	"net/http"

	taskdto "github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ValidateMappedRequest validates Provider-model capabilities after the shared
// model_mapping authority has selected the exact Provider model, but before
// pricing, hold creation, or Provider I/O.
func (a *TaskAdaptor) ValidateMappedRequest(c *gin.Context, info *relaycommon.RelayInfo) *taskdto.TaskError {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return service.TaskErrorWrapperLocal(errors.New("Seedance channels require the ModelArk V3 request contract"), "invalid_video_contract", http.StatusBadRequest)
	}

	if SeedanceExtensionProtocolMigrated(a.protocol) {
		if taskErr := service.PrepareVideoReferenceAudio(c, info); taskErr != nil {
			return taskErr
		}
		if a.protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			if err := validateSynlinkMedia(contract.ModelArk); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_video_parameter", http.StatusBadRequest)
			}
		}
		if a.protocol == dto.VideoUpstreamProtocolMoxingModelArkV1 || a.protocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 || a.protocol == dto.VideoUpstreamProtocolModelArkV3CMCC || a.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 || a.protocol == dto.VideoUpstreamProtocolSynlinkVideoV1 {
			if _, err := a.ensureSeedanceCreateConversion(c, info); err != nil {
				return service.TaskErrorWrapperLocal(err, "invalid_video_parameter", http.StatusBadRequest)
			}
		}
		return nil
	}
	return service.TaskErrorWrapperLocal(errors.New("video protocol is not registered for new requests"), "invalid_video_parameter", http.StatusBadRequest)
}
