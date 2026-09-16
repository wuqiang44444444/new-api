package openai

import "github.com/QuantumNous/new-api/relaykit/dto"

// normalizeOpenAIUsage maps the OpenAI Images usage shape (input_tokens /
// output_tokens / input_tokens_details / output_tokens_details) onto the
// canonical prompt/completion fields. It is shared verbatim by every image
// usage boundary — client JSON, client SSE, background JSON and background
// SSE (ImageTaskUsage) — so the same usage payload bills identically in sync
// and task execution. The image API never returns prompt_tokens /
// completion_tokens, so the overwrite (=) semantics here are equivalent to
// the previous additive (+=) behavior while avoiding any future
// double-counting if both field sets are ever populated. Do not reuse this on
// chat/embedding paths without revisiting the overwrite semantics.
//
// Detail objects are upstream evidence and map by whole-object overwrite.
// When output_tokens_details is present it is the image protocol's
// authoritative output breakdown and takes priority over a decoded
// completion_tokens_details; when absent, completion_tokens_details is kept
// as decoded. Missing details are never inferred from the totals, and
// duplicate representations of the same tokens are never added together.
func normalizeOpenAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.CachedCreationTokens = usage.InputTokensDetails.CachedCreationTokens
		usage.PromptTokensDetails.CacheWriteTokens = usage.InputTokensDetails.CacheWriteTokens
		usage.PromptTokensDetails.ImageTokens = usage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.TextTokens = usage.InputTokensDetails.TextTokens
		usage.PromptTokensDetails.AudioTokens = usage.InputTokensDetails.AudioTokens
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails.TextTokens = usage.OutputTokensDetails.TextTokens
		usage.CompletionTokenDetails.AudioTokens = usage.OutputTokensDetails.AudioTokens
		usage.CompletionTokenDetails.ImageTokens = usage.OutputTokensDetails.ImageTokens
		usage.CompletionTokenDetails.ReasoningTokens = usage.OutputTokensDetails.ReasoningTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}
