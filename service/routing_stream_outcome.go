package service

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func relayStreamSucceeded(info *relaycommon.RelayInfo, requireTerminal bool) bool {
	if info == nil || info.StreamStatus == nil {
		return !requireTerminal
	}
	outcome := info.StreamStatus.OutcomeSnapshot()
	if !outcome.IsNormalEnd() || outcome.HasErrors {
		return false
	}
	switch outcome.Response {
	case relaycommon.ResponseOutcomeCompleted:
		return true
	case relaycommon.ResponseOutcomeUnknown:
		return !requireTerminal
	case relaycommon.ResponseOutcomeIncomplete:
		// The Claude and Gemini Responses adapters previously marked these
		// normal upstream stops completed. Preserve their routing semantics
		// while recording the converted protocol outcome accurately.
		upstream := info.GetFinalRequestRelayFormat()
		return info.RelayFormat == types.RelayFormatOpenAIResponses &&
			(upstream == types.RelayFormatClaude || upstream == types.RelayFormatGemini) &&
			outcome.IncompleteReason == "max_output_tokens"
	default:
		return false
	}
}
