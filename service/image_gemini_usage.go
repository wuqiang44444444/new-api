package service

import (
	"errors"
	"math"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// ValidateGeminiImageUsage keeps absent modality evidence distinct from zero.
// Lite has different text/image output prices; it must not settle unclassified
// output as text. This does not estimate tokens from pixels or image count.
func ValidateGeminiImageUsage(providerModel string, usage *dto.Usage) error {
	if providerModel != "gemini-3.1-flash-lite-image" {
		return nil
	}
	invalid := errors.New("gemini image billing usage is incomplete or inconsistent")
	if usage == nil {
		return invalid
	}
	details := usage.CompletionTokenDetails
	for _, count := range []int{usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens,
		usage.PromptTokensDetails.CachedTokens, details.ImageTokens, details.TextTokens, details.ReasoningTokens, details.AudioTokens} {
		if count < 0 || int64(count) > math.MaxInt32 {
			return invalid
		}
	}
	if details.ImageTokens == 0 || details.AudioTokens != 0 ||
		usage.PromptTokensDetails.CachedTokens > usage.PromptTokens ||
		int64(details.ImageTokens)+int64(details.TextTokens)+int64(details.ReasoningTokens) != int64(usage.CompletionTokens) ||
		int64(usage.PromptTokens)+int64(usage.CompletionTokens) != int64(usage.TotalTokens) {
		return invalid
	}
	return nil
}
