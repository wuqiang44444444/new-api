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
	if a.protocol == dto.VideoUpstreamProtocolModelArkV3CMCC {
		if err := validateCMCCProviderModelRequest(info.UpstreamModelName, contract.ModelArk); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_video_parameter", http.StatusBadRequest)
		}
	} else if a.protocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 ||
		a.protocol == dto.VideoUpstreamProtocolMoxingMediaTaskV1 ||
		a.protocol == dto.VideoUpstreamProtocolMoxingModelArkV1 ||
		a.protocol == dto.VideoUpstreamProtocolFunCloudSeedance {
		if err := validateProviderModelRequest(a.protocol, info.UpstreamModelName, contract.ModelArk); err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_video_parameter", http.StatusBadRequest)
		}
	}
	if a.protocol == dto.VideoUpstreamProtocolFunCloudSeedance {
		createPath, queryPath := a.protocol.TransportPaths(info.UpstreamModelName)
		if createPath == "" || queryPath == "" {
			return service.TaskErrorWrapperLocal(
				errors.New("the selected customer model has no registered transport path"),
				"invalid_video_parameter",
				http.StatusBadRequest,
			)
		}
		a.createPath = createPath
		info.ChannelOtherSettings.VideoUpstreamCreatePath = createPath
		info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate = queryPath
	}
	return nil
}
