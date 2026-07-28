package doubao

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// applyVideoServiceTierPolicy enforces the selected DoubaoVideo channel's
// service-tier capability before pricing and upstream submission.
func applyVideoServiceTierPolicy(c *gin.Context, info *relaycommon.RelayInfo, profile dto.VideoUpstreamProfile) *dto.TaskError {
	if contract, ok := relaycommon.GetVideoContractRequest(c); ok && contract.ModelArk != nil {
		if contract.ModelArk.ServiceTier == nil {
			return nil
		}
		tier := strings.TrimSpace(*contract.ModelArk.ServiceTier)
		if tier == "" {
			contract.ModelArk.ServiceTier = nil
			relaycommon.SetVideoContractRequest(c, contract)
			return nil
		}
		allowServiceTier := info != nil && info.ChannelOtherSettings.AllowServiceTier
		if tier == "default" && (profile == dto.VideoUpstreamProfileThirdPartyRelay || !allowServiceTier) {
			contract.ModelArk.ServiceTier = nil
			relaycommon.SetVideoContractRequest(c, contract)
			return nil
		}
		if tier == "flex" && (profile == dto.VideoUpstreamProfileThirdPartyRelay || !allowServiceTier) {
			return service.TaskErrorWrapperLocal(
				fmt.Errorf("service_tier %q is not supported by the selected video channel", tier),
				"unsupported_parameter",
				http.StatusBadRequest,
			)
		}
		return nil
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if req.Metadata == nil {
		return nil
	}

	value, exists := req.Metadata["service_tier"]
	if !exists {
		return nil
	}
	if value == nil {
		delete(req.Metadata, "service_tier")
		return nil
	}

	tier, ok := value.(string)
	if !ok {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("service_tier must be a string"),
			"invalid_request",
			http.StatusBadRequest,
		)
	}
	tier = strings.TrimSpace(tier)
	if tier == "" {
		delete(req.Metadata, "service_tier")
		return nil
	}
	if tier != "default" && tier != "flex" {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("service_tier %q is not supported", tier),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}

	allowServiceTier := info != nil && info.ChannelOtherSettings.AllowServiceTier
	if tier == "default" && (profile == dto.VideoUpstreamProfileThirdPartyRelay || !allowServiceTier) {
		delete(req.Metadata, "service_tier")
		return nil
	}
	if profile == dto.VideoUpstreamProfileThirdPartyRelay || !allowServiceTier {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("service_tier %q is not supported by the selected video channel", tier),
			"unsupported_parameter",
			http.StatusBadRequest,
		)
	}

	req.Metadata["service_tier"] = tier
	return nil
}
