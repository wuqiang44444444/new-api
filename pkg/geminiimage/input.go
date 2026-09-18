package geminiimage

import "github.com/QuantumNous/new-api/constant"

// InlineImageLimit is a provider transport limit, not a routing registration.
// Zero means the existing common image budget remains authoritative.
func InlineImageLimit(model string, channelType int) int {
	if model == "gemini-3.1-flash-lite-image" && channelType == constant.ChannelTypeVertexAi {
		return 7_000_000
	}
	return 0
}
