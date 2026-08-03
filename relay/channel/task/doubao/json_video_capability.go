package doubao

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao/thirdparty/jsonvideo"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func buildJSONVideoCreateRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	profile taskdto.VideoUpstreamProfile,
) ([]byte, bool, error) {
	if profile != taskdto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference {
		return nil, false, nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ContractID != taskdto.VideoContractModelArkV3 || contract.ModelArk == nil {
		return nil, true, fmt.Errorf("JSON video profile requires a ModelArk request")
	}
	upstreamModel := strings.TrimSpace(contract.ModelArk.Model)
	if info != nil && info.IsModelMapped {
		upstreamModel = strings.TrimSpace(info.UpstreamModelName)
	} else if info != nil {
		info.UpstreamModelName = upstreamModel
	}
	capability, ok := common.GetContextKeyType[model.VideoSKUCapability](
		c,
		constant.ContextKeyResolvedVideoSKUCapability,
	)
	if !ok {
		return nil, true, fmt.Errorf("JSON video capability snapshot is unavailable")
	}
	body, err := jsonvideo.CreateRequest(contract.ModelArk, upstreamModel, capability)
	return body, true, err
}
