package common

import (
	"strings"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// UsesSynchronousTextCancellation scopes the response-header billing boundary
// to the native synchronous text entries.
func (info *RelayInfo) UsesSynchronousTextCancellation() bool {
	if info.RelayFormat == types.RelayFormatClaude {
		return true
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeResponses:
		return true
	case relayconstant.RelayModeGemini:
		path, _, _ := strings.Cut(info.RequestURLPath, "?")
		return strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent")
	default:
		return false
	}
}
