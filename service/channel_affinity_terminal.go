package service

import (
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func ObserveChannelAffinityNonStream(c *gin.Context, complete bool) {
	if r := failoverRequestFrom(c); r != nil {
		r.nonStreamObserved, r.nonStreamTerminal = true, complete
	}
}

func ObserveChannelAffinityChatCompletion(c *gin.Context, info *relaycommon.RelayInfo, choices []dto.OpenAITextResponseChoice) {
	r := failoverRequestFrom(c)
	if r == nil || r.operation != "chat" {
		return
	}
	tracker := affinityStreamTracker(c)
	if tracker == nil {
		tracker = &r.stream
	}
	count := tracker.chatChoiceCount(c, info)
	complete := count > 0 && count == len(choices)
	if complete {
		seen := make(map[int]bool, count)
		for _, choice := range choices {
			if choice.Index < 0 || choice.Index >= count || seen[choice.Index] || choice.FinishReason == "" {
				complete = false
				break
			}
			seen[choice.Index] = true
		}
	}
	ObserveChannelAffinityNonStream(c, complete)
}

// ObserveNativeResponsesResult accepts only a completed direct upstream
// Responses result as terminal and ownership evidence.
func ObserveNativeResponsesResult(c *gin.Context, response *dto.OpenAIResponsesResponse, header http.Header) {
	complete := string(response.Status) == `"completed"` && response.ID != ""
	ObserveChannelAffinityNonStream(c, complete)
	if complete {
		ObserveNativeResponseOwner(c, response.ID, header.Get("X-Codex-Turn-State"))
	}
}

func ObserveNativeResponsesStreamEvent(c *gin.Context, event *dto.ResponsesStreamResponse, header http.Header) {
	if event.Type == "response.completed" && event.Response != nil {
		ObserveNativeResponseOwner(c, event.Response.ID, header.Get("X-Codex-Turn-State"))
	}
}
