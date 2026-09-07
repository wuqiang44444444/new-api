package relay

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// V3 documents these HTTP statuses as rejected creation requests. Require its
// structured error and no task ID; HTML proxy errors and ambiguous successes
// must retain their hold and must not be retried automatically.
func isFunCloudModelArkCreateRejection(info *relaycommon.RelayInfo, status int, body []byte) bool {
	if info == nil || info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeSeedanceLink || info.ChannelOtherSettings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		return false
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden:
	default:
		return false
	}
	var response struct {
		ID    any `json:"id"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &response) != nil || response.ID != nil || response.Error == nil {
		return false
	}
	return strings.TrimSpace(response.Error.Code) != "" && strings.TrimSpace(response.Error.Message) != ""
}
