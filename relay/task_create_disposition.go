package relay

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func markAmbiguousTaskCreate(c *gin.Context, info *relaycommon.RelayInfo) {
	service.MarkTaskCreateAttemptOutcomeUnknown(c, info)
}

func taskCreateHTTPDisposition(
	info *relaycommon.RelayInfo,
	status int,
	providerCode string,
) relaycommon.TaskCreateDisposition {
	if info != nil && info.ChannelMeta != nil &&
		info.OriginModelName == model.VideoSKUDoubaoSeedance20260128 &&
		info.ChannelOtherSettings.VideoUpstreamProfile == dto.VideoUpstreamProfileThirdPartyRelay &&
		status == http.StatusForbidden && providerCode == "user_quota_insufficient" {
		return relaycommon.TaskCreateTerminalRejection
	}
	// Unregistered status + provider-code combinations remain unknown. A new
	// terminal rejection requires exact provider evidence and a regression test.
	return relaycommon.TaskCreateOutcomeUnknown
}
