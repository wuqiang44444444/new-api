package seedance

import (
	"fmt"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/funcloud"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func buildFunCloudVideoCreateRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	profile taskdto.VideoUpstreamProfile,
) ([]byte, bool, error) {
	if profile != taskdto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2 {
		return nil, false, nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, true, fmt.Errorf("FunCloud profile requires a ModelArk request")
	}
	if info != nil {
		info.UpstreamModelName = ""
	}
	body, err := funcloud.CreateRequest(contract.ModelArk)
	return body, true, err
}
