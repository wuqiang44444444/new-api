package service

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Preserve the semantic already used by native settlement. In particular, its
// legacy Claude conversion branch prices prompt tokens excluding cache.
func appendTextStatementUsage(other *model.LogOther, info *relaycommon.RelayInfo, usage *dto.Usage, summary textQuotaSummary) {
	semantic := summary.UsageSemantic
	if isLegacyClaudeDerivedOpenAIUsage(info, usage) {
		semantic = "anthropic"
	}
	other.SetPublic("usage_semantic", semantic)
}

// Images have already normalized OpenAI input/output fields before Task
// persistence. Project those frozen facts without recalculating any charge.
func appendImageStatementUsage(other *model.LogOther, usage *dto.Usage) {
	other.SetPublic("usage_semantic", "openai")
	other.SetPublic("input_tokens_total", max(usage.PromptTokens, 0))
	other.SetPublic("cache_tokens", max(usage.PromptTokensDetails.CachedTokens, 0))
	other.SetPublic("cache_write_tokens", usage.PromptTokensDetails.CacheCreationTokensTotal())
}
