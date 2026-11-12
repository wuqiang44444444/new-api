package service

import (
	"strconv"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ObserveDefaultAffinityChatRequest reads the effective n from the overridden
// upstream body. The channel metadata is rebuilt for each relay attempt.
func ObserveDefaultAffinityChatRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) {
	if c.Request.URL.Path != "/v1/chat/completions" || info.GetFinalRequestRelayFormat() != types.RelayFormatOpenAI {
		return
	}
	state, _ := c.Get(defaultAffinityContextKey)
	affinity, ok := state.(*defaultAffinityRequest)
	if !ok {
		return
	}
	affinity.chatOverrideAttempt = info.ChannelMeta
	affinity.chatOverrideChoices = 1
	if n := gjson.GetBytes(body, "n"); n.Exists() {
		count, err := strconv.Atoi(n.Raw)
		if err != nil || count <= 0 {
			affinity.chatOverrideChoices = 0
			return
		}
		affinity.chatOverrideChoices = count
	}
}

func (affinity *defaultAffinityRequest) chatChoiceCount(c *gin.Context, info *relaycommon.RelayInfo) int {
	if c.Request.URL.Path == "/v1/chat/completions" {
		if affinity.chatOverrideAttempt != nil && affinity.chatOverrideAttempt == info.ChannelMeta {
			return affinity.chatOverrideChoices
		}
		if request, ok := info.Request.(*dto.GeneralOpenAIRequest); ok && request.N != nil && *request.N > 1 {
			return *request.N
		}
	}
	return 1
}

// ObserveDefaultAffinityChatChoice records finish reasons for a multi-choice
// request without changing the shared outcome used by health and billing.
func ObserveDefaultAffinityChatChoice(c *gin.Context, info *relaycommon.RelayInfo, index int) {
	if c == nil {
		return
	}
	state, _ := c.Get(defaultAffinityContextKey)
	affinity, ok := state.(*defaultAffinityRequest)
	if !ok {
		return
	}
	count := affinity.chatChoiceCount(c, info)
	if count <= 1 || index < 0 || index >= count {
		return
	}
	if affinity.chatChoiceStream != info.StreamStatus {
		affinity.chatChoiceStream = info.StreamStatus
		affinity.finishedChatChoices = make(map[int]struct{})
	}
	affinity.finishedChatChoices[index] = struct{}{}
}
