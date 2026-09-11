package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// annotateChannelTestCacheUsage marks only provably missing native test data.
// This is a response projection; the persisted log remains unchanged.
func annotateChannelTestCacheUsage(logs []*Log) {
	for _, log := range logs {
		if !isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content) {
			continue
		}
		parsed := parseBillingReconciliationLog(billingReconciliationLog{Type: log.Type, TokenId: log.TokenId, TokenName: log.TokenName, Content: log.Content, ModelName: log.ModelName, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Other: log.Other})
		if !parsed.cacheWriteUnavailable {
			continue
		}
		var other map[string]json.RawMessage
		if common.UnmarshalJsonStr(log.Other, &other) != nil || other == nil {
			other = make(map[string]json.RawMessage)
		}
		other["cache_write_unavailable"] = json.RawMessage("true")
		if data, err := common.Marshal(other); err == nil {
			log.Other = string(data)
		}
	}
}
