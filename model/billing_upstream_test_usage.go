package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// Test logs freeze the response conversion used by their writer. Never use a
// model name, current channel type or today's pricing to recover that semantic.
func historicalTestUsageSemantic(other map[string]json.RawMessage) string {
	if semantic := billingBreakdownString(other["usage_semantic"]); semantic != "" {
		return semantic
	}
	var chain []string
	if common.Unmarshal(other["request_conversion"], &chain) != nil || len(chain) == 0 {
		return ""
	}
	switch chain[len(chain)-1] {
	case "OpenAI Compatible", "OpenAI Responses":
		return "openai"
	case "Claude Messages":
		return "anthropic"
	case "Google Gemini":
		return "gemini"
	case "embedding":
		return "embedding"
	case "openai_image":
		return "openai"
	}
	return ""
}

// Gemini generation usage maps cachedContentTokenCount to cache reads, but has
// no cache-creation token meter in the canonical billing usage. Its absent write
// dimension contributes zero to this request's platform pricing. This does not
// describe external cache storage charges. Embedding probes similarly bill only
// their input tokens. Explicit evidence always wins over either omission rule.
func historicalTestOmitsCacheWrite(log billingReconciliationLog, other map[string]json.RawMessage) bool {
	if !isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content) {
		return false
	}
	semantic := historicalTestUsageSemantic(other)
	if semantic != "gemini" && !(semantic == "embedding" && billingBreakdownString(other["request_path"]) == "/v1/embeddings" && log.CompletionTokens == 0) {
		return false
	}
	for _, key := range []string{"cache_write_tokens_reported", "cache_write_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h"} {
		if _, present := other[key]; present {
			return false
		}
	}
	return true
}
