package relay

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// minimaxCreateDisposition classifies a non-2xx JD task submit response. The
// documented JD error envelope ({error:{code,message,status}, result:null})
// with no task id is an explicit not-created rejection: the attempt hold is
// released. Everything else — timeouts, malformed bodies, contradictory
// envelopes — stays unknown and keeps the 24h client funds warranty.
func minimaxCreateDisposition(status int, body []byte) relaycommon.TaskCreateDisposition {
	var response struct {
		TaskID any `json:"task_id"`
		Result any `json:"result"`
		Error  *struct {
			Code    any     `json:"code"`
			Message string  `json:"message"`
			Status  *string `json:"status"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &response) != nil {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	if response.TaskID != nil || response.Result != nil || response.Error == nil {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	// Only a client-error rejection is definitive; timeouts and rate limits
	// leave the creation unknown.
	if status < 400 || status >= 500 || status == http.StatusRequestTimeout ||
		status == http.StatusTooEarly || status == http.StatusTooManyRequests {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	return relaycommon.TaskCreateTerminalRejection
}
