package model

import "github.com/QuantumNous/new-api/common"

// taskLogQuotaData uses the statement event classification for every projection.
// A monetary adjustment is not another request; a refund is a signed amount.
func taskLogQuotaData(log *Log, nodeName string) *QuotaData {
	parsed := parseBillingReconciliationLog(billingReconciliationLog{Type: log.Type, Quota: log.Quota,
		CompletionTokens: log.CompletionTokens, PromptTokens: log.PromptTokens, Other: log.Other})
	count := common.QuotaFromFloat(float64(billingStatementRequestCount(parsed)))
	quota := log.Quota
	if log.Type == LogTypeRefund {
		quota = -quota
	}
	if nodeName == "" {
		nodeName = common.NodeName
	}
	tokens := log.PromptTokens + log.CompletionTokens
	if parsed.isRefund {
		tokens = 0
	}
	return &QuotaData{UserID: log.UserId, Username: log.Username, ModelName: log.ModelName, CreatedAt: log.CreatedAt - log.CreatedAt%3600,
		UseGroup: log.Group, TokenID: log.TokenId, ChannelID: log.ChannelId, NodeName: nodeName, Count: count, Quota: quota,
		TokenUsed: tokens}
}

func recordTaskQuotaData(log *Log, nodeName string) {
	if !common.DataExportEnabled {
		return
	}
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(taskLogQuotaData(log, nodeName))
}
