package seedance

import (
	"fmt"
	"strings"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func buildFeicaiVideoCreateRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	profile taskdto.VideoUpstreamProfile,
) ([]byte, bool, error) {
	if profile != taskdto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		return nil, false, nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, true, fmt.Errorf("the selected video adapter requires a ModelArk request")
	}
	upstreamModel := strings.TrimSpace(contract.ModelArk.Model)
	if info != nil && info.IsModelMapped {
		upstreamModel = strings.TrimSpace(info.UpstreamModelName)
	} else if info != nil {
		info.UpstreamModelName = upstreamModel
	}
	body, err := feicai.CreateRequest(contract.ModelArk, upstreamModel)
	return body, true, err
}
