package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestConvertedResponsesOutputLimitRemainsRoutingSuccess(t *testing.T) {
	for _, tc := range []struct {
		name     string
		upstream types.RelayFormat
		end      relaycommon.StreamEndReason
		reason   string
		want     bool
	}{
		{"Claude output limit", types.RelayFormatClaude, relaycommon.StreamEndReasonEOF, "max_output_tokens", true},
		{"Gemini output limit", types.RelayFormatGemini, relaycommon.StreamEndReasonEOF, "max_output_tokens", true},
		{"OpenAI passthrough keeps its existing policy", types.RelayFormatOpenAIResponses, relaycommon.StreamEndReasonEOF, "max_output_tokens", false},
		{"filter is not a routing success", types.RelayFormatGemini, relaycommon.StreamEndReasonEOF, "content_filter", false},
		{"client disconnected", types.RelayFormatClaude, relaycommon.StreamEndReasonClientGone, "max_output_tokens", false},
		{"upstream timed out", types.RelayFormatGemini, relaycommon.StreamEndReasonTimeout, "max_output_tokens", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := relaycommon.NewStreamStatus()
			stream.MarkIncomplete(tc.reason)
			stream.SetEndReason(tc.end, nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatOpenAIResponses, FinalRequestRelayFormat: tc.upstream, StreamStatus: stream}
			assert.Equal(t, tc.want, relayStreamSucceeded(info, true))
			assert.Equal(t, tc.want, relayStreamSucceeded(info, false))
			assert.Equal(t, string(relaycommon.ResponseOutcomeIncomplete), stream.ResponseOutcome())
		})
	}
}
