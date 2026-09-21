package channel

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// Preserve the immediate gateway ID when present; direct Azure and OpenAI
// responses use their documented request headers. Never substitute a local ID.
func upstreamResponseRequestID(header http.Header, channelType int) string {
	value := header.Get(common.RequestIdKey)
	if value == "" && channelType == constant.ChannelTypeAzure {
		value = header.Get("apim-request-id")
	}
	if value == "" {
		value = header.Get("x-request-id")
	}
	value = strings.TrimSpace(value)
	// The log column is varchar(128). Reject unusable evidence instead of
	// truncating it into a different request identity or failing settlement.
	if len(value) > 128 || strings.IndexFunc(value, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return ""
	}
	return value
}
