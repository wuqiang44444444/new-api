package model

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// The native text/image settlement writer persists positive cache writes only.
// Absence in that writer's log means zero in the platform's billing usage;
// it is not a lost meter. Channel tests and tasks have different
// contracts and must not borrow this interpretation. Explicit missing/malformed
// evidence also wins over historical omission semantics.
func nativeUsageLogOmitsZeroCacheWrite(log billingReconciliationLog, other map[string]json.RawMessage) bool {
	if log.Type != LogTypeConsume || isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content) {
		return false
	}
	for _, key := range []string{"cache_write_tokens_reported", "cache_write_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h", "task_id", "is_task"} {
		if _, present := other[key]; present {
			return false
		}
	}
	path := billingBreakdownString(other["request_path"])
	text := path == "/v1/chat/completions" || path == "/v1/responses" || path == "/v1/messages" || path == "/pg/chat/completions" ||
		path == "/v1/images/generations" || path == "/v1/images/edits" ||
		(strings.HasPrefix(path, "/v1beta/models/") && (strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent")))
	if !text {
		return false
	}
	if claude, _ := billingReconciliationBool(other["claude"]); claude || billingBreakdownString(other["usage_semantic"]) == "anthropic" {
		// GenerateClaudeOtherInfo always writes cache_creation_tokens, even zero.
		return false
	}
	// GenerateTextOtherInfo's frozen price and cache fields identify its log
	// contract; a bare URL or a model name cannot establish writer semantics.
	for _, key := range []string{"model_ratio", "group_ratio", "completion_ratio", "cache_tokens", "cache_ratio", "model_price", "user_group_ratio"} {
		if _, valid := billingReconciliationFloat(other[key]); !valid {
			return false
		}
	}
	return true
}

// Task expressions which only bill output or their own named meters do not
// have a prompt-cache charge. Missing unused cache dimensions are not missing
// billing facts. This uses the frozen expression, never the current model.
func applyUpstreamTaskCacheApplicability(log billingReconciliationLog, other, snapshot map[string]json.RawMessage, parsed *parsedBillingReconciliationLog) {
	if (!parsed.isTask && parsed.taskID == "") || parsed.hasImageCount || parsed.billingMode != BillingReconciliationModeToken {
		return
	}
	encoded := billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64"))
	if encoded == "" {
		// Native token-task adjustment logs record total tokens in completion_tokens;
		// their writer has no input-cache meter or charge (RecalculateTaskQuota).
		_, actual := billingBreakdownNonNegativeInt(other["actual_quota"])
		_, pre := billingBreakdownNonNegativeInt(other["pre_consumed_quota"])
		if parsed.taskID == "" || !actual || !pre || log.PromptTokens != 0 || log.CompletionTokens <= 0 || parsed.modelPrice == nil || *parsed.modelPrice != 0 {
			return
		}
	} else {
		expression, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return
		}
		vars := billingexpr.UsedVars(string(expression))
		if vars == nil {
			return
		}
		for _, name := range []string{"p", "len", "cr", "cc", "cc1h"} {
			if vars[name] {
				return
			}
		}
	}
	for _, key := range []string{"cache_tokens", "cache_write_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h", "cache_read_tokens_reported", "cache_write_tokens_reported"} {
		if _, exists := other[key]; exists {
			return
		}
	}
	parsed.cacheReadEvidence = upstreamCacheRecorded
	parsed.cacheWriteEvidence = upstreamCacheRecorded
}
