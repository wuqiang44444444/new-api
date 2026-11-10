package relay

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func nexhinaCreateDisposition(status int, body []byte) relaycommon.TaskCreateDisposition {
	if status != http.StatusUnauthorized {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	// Match the verified Nexhina pre-admission response. Extra fields can
	// carry task acceptance evidence and leave the outcome unknown.
	var response map[string]map[string]*string
	if common.Unmarshal(body, &response) != nil || len(response) != 1 || len(response["error"]) != 3 {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	detail := response["error"]
	code, kind, message := detail["code"], detail["type"], detail["message"]
	if code == nil || *code != "" || kind == nil || *kind != "nexhina_error" || message == nil || strings.TrimSpace(*message) == "" {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	return relaycommon.TaskCreateTerminalRejection
}
