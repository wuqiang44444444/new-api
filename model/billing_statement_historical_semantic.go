package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// Recovery follows the historical writers, not the model or current channel.
// See docs/80-dev/2026-09-18-账单历史解析与持款投影补正.md for writer evidence.
func recoverBillingBreakdownInputSemantic(other map[string]json.RawMessage, cacheReadTokens, promptTokens int64) string {
	if promptTokens <= 0 || len(other["input_tokens_total"]) > 0 || len(other["usage_semantic"]) > 0 {
		return ""
	}
	var admin map[string]json.RawMessage
	if common.Unmarshal(other["admin_info"], &admin) != nil {
		return ""
	}
	path := billingBreakdownString(admin["usage_billing_path"])
	if path == "billing-usage-gemini" {
		if cacheReadTokens > promptTokens {
			return ""
		}
		return "gemini"
	}
	if path != "upstream" {
		return ""
	}
	var chain []string
	if common.Unmarshal(other["request_conversion"], &chain) != nil || len(chain) == 0 {
		return ""
	}
	first := map[string]string{"/v1/chat/completions": "OpenAI Compatible", "/v1/responses": "OpenAI Responses", "/v1/messages": "Claude Messages"}[billingBreakdownString(other["request_path"])]
	if first == "" || chain[0] != first {
		return ""
	}
	for _, format := range chain {
		switch format {
		case "OpenAI Compatible", "OpenAI Responses", "Claude Messages", "Google Gemini":
		default:
			return ""
		}
	}
	claude := false
	if raw := other["claude"]; len(raw) > 0 {
		var valid bool
		claude, valid = billingReconciliationBool(raw)
		if !valid {
			return ""
		}
	}
	for _, key := range []string{"cache_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h", "cache_write_tokens"} {
		if raw := other[key]; len(raw) > 0 {
			if _, ok := billingBreakdownNonNegativeInt(raw); !ok {
				return ""
			}
		}
	}
	final := chain[len(chain)-1]
	if final == "Claude Messages" {
		// GenerateClaudeOtherInfo certifies the final Claude writer. The prompt
		// excludes cache, so a cache read larger than prompt is valid here.
		if claude {
			return "anthropic"
		}
		return ""
	}
	if final != "OpenAI Compatible" && final != "OpenAI Responses" {
		return ""
	}
	if claude || cacheReadTokens > promptTokens {
		return ""
	}
	// Split TTL fields may come from legacy Claude-derived OpenAI usage. The
	// historical log did not always retain UsageSource, so do not guess its branch.
	for _, key := range []string{"cache_creation_tokens_5m", "cache_creation_tokens_1h"} {
		if raw := other[key]; len(raw) > 0 {
			n, ok := billingBreakdownNonNegativeInt(raw)
			if !ok || n > 0 {
				return ""
			}
		}
	}
	for _, key := range []string{"cache_creation_tokens", "cache_write_tokens"} {
		if raw := other[key]; len(raw) > 0 {
			n, ok := billingBreakdownNonNegativeInt(raw)
			if !ok || n > promptTokens-cacheReadTokens {
				return ""
			}
		}
	}
	// NormalizeResponsesUsage copies input_tokens into PromptTokens. Both this
	// writer and ordinary Chat retain cache creation as a subset of the prompt.
	return "openai"
}
