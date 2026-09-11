package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// appendChannelTestCacheUsage preserves the response facts used to price a
// channel test. Logging must not discard writes simply because the test uses
// GenerateTextOtherInfo for every response format.
func appendChannelTestCacheUsage(other *model.LogOther, usage *dto.Usage) {
	creationTokens := usage.PromptTokensDetails.CacheCreationTokensTotal()
	other.SetPublic("cache_creation_tokens", creationTokens)
	other.SetPublic("cache_creation_tokens_5m", usage.ClaudeCacheCreation5mTokens)
	other.SetPublic("cache_creation_tokens_1h", usage.ClaudeCacheCreation1hTokens)
	other.SetPublic("cache_write_tokens", max(creationTokens, usage.ClaudeCacheCreation5mTokens+usage.ClaudeCacheCreation1hTokens))
	if usage.UsageSemantic != "" {
		other.SetPublic("usage_semantic", usage.UsageSemantic)
	}
}
