package seedance

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// applyVideoServiceTierPolicy enforces the selected Seedance channel's
// service-tier capability before pricing and upstream submission.
func applyVideoServiceTierPolicy(c *gin.Context, info *relaycommon.RelayInfo, profile dto.VideoUpstreamProfile) *dto.TaskError {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil || contract.ModelArk.ServiceTier == nil {
		return nil
	}
	tier := strings.TrimSpace(*contract.ModelArk.ServiceTier)
	if tier == "" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("service_tier must not be empty"),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}

	allowServiceTier := info != nil && info.ChannelOtherSettings.AllowServiceTier
	if profile == dto.VideoUpstreamProfileThirdPartyRelay || !allowServiceTier {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("service_tier %q is not supported by the selected video channel", tier),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}
	contract.ModelArk.ServiceTier = common.GetPointer(tier)
	relaycommon.SetVideoContractRequest(c, contract)
	return nil
}
