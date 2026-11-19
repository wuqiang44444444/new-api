package relay

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func isVerifiedMoxingDraftRejection(info *relaycommon.RelayInfo, status int, body []byte) bool {
	if info.ChannelOtherSettings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolMoxingModelArkV1 ||
		status != http.StatusBadRequest || !info.TaskRelayInfo.IsVideoDraftOperation() {
		return false
	}
	type rejection struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	var response struct {
		ID     any `json:"id"`
		TaskID any `json:"task_id"`
		Data   any `json:"data"`
		rejection
		Error *rejection `json:"error"`
	}
	if common.Unmarshal(body, &response) != nil || response.ID != nil || response.TaskID != nil || response.Data != nil {
		return false
	}
	detail := response.rejection
	if response.Error != nil {
		switch string(response.Code) {
		case "", "null", "400", `"invalid_request_error"`:
		default:
			return false
		}
		detail = *response.Error
	}
	message := strings.TrimSpace(detail.Message)
	if string(detail.Code) == `"invalid_request_error"` {
		return message != ""
	}
	switch string(detail.Code) {
	case "", "null", "400":
		return message == "请求参数不合法，请检查后重试。"
	default:
		return false
	}
}
