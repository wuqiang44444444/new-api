package relay

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// Only the documented V2 pre-admission errors prove that no task was created.
func minimaxOfficialCreateDisposition(status int, body []byte) relaycommon.TaskCreateDisposition {
	var expectedType string
	switch status {
	case http.StatusBadRequest:
		expectedType = "bad_request_error"
	case http.StatusUnauthorized:
		expectedType = "authorized_error"
	case http.StatusPaymentRequired:
		expectedType = "insufficient_balance_error"
	case http.StatusUnprocessableEntity:
		expectedType = "unprocessable_entity_error"
	default:
		return relaycommon.TaskCreateOutcomeUnknown
	}
	var response struct {
		Type   string `json:"type"`
		ID     any    `json:"id"`
		TaskID any    `json:"task_id"`
		Task   any    `json:"task"`
		Data   any    `json:"data"`
		Result any    `json:"result"`
		Error  struct {
			Type     string `json:"type"`
			Message  string `json:"message"`
			HTTPCode string `json:"http_code"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &response) != nil || response.Type != "error" || response.ID != nil ||
		response.TaskID != nil || response.Task != nil || response.Data != nil || response.Result != nil ||
		response.Error.Type != expectedType || response.Error.HTTPCode != strconv.Itoa(status) ||
		strings.TrimSpace(response.Error.Message) == "" {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	return relaycommon.TaskCreateTerminalRejection
}
