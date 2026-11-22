package seedance

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// EstimateTaskPreConsumeTokens estimates usage for an explicitly sized,
// fixed-duration request without video input. A zero estimate leaves the
// configured budget in use. This estimate only prices the initial hold.
func (a *TaskAdaptor) EstimateTaskPreConsumeTokens(c *gin.Context, info *relaycommon.RelayInfo, probe map[string]any) int {
	if a.protocol != dto.VideoUpstreamProtocolModelArkV3Volcengine || info.TaskRelayInfo.IsVideoDraftOperation() {
		return 0
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return 0
	}
	request := contract.ModelArk
	if request.Duration == nil || request.Frames != nil || request.Resolution == nil ||
		strings.TrimSpace(*request.Resolution) == "" || probe["has_video_input"] != false {
		return 0
	}

	maxDuration := 15
	switch providerModelFromRelayInfo(info, request.Model) {
	case modelSeedance20, modelSeedance20Fast, modelSeedance20Mini:
	case modelSeedance25:
		maxDuration = 30
	default:
		return 0
	}
	duration := *request.Duration
	if duration < 4 || duration > maxDuration {
		return 0
	}

	// Use the largest published pixel area for each resolution across the
	// official Seedance 2.x aspect ratios. Thirty frames per second leaves
	// headroom over the published pricing examples (24 fps).
	var pixels int64
	switch probe["resolution"] {
	case "480p":
		pixels = 992 * 432
	case "720p":
		pixels = 1112 * 834
	case "1080p":
		pixels = 2206 * 946
	case "4k":
		pixels = 3326 * 2494
	default:
		return 0
	}
	return int((pixels*30*int64(duration) + 1023) / 1024)
}
