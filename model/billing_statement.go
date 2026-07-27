package model

import "sort"

type BillingStatementItem struct {
	TokenId                int     `json:"token_id"`
	TokenName              string  `json:"token_name"`
	ModelName              string  `json:"model_name"`
	Requests               int64   `json:"requests"`
	PromptTokens           int64   `json:"prompt_tokens"`
	CompletionTokens       int64   `json:"completion_tokens"`
	TotalTokens            int64   `json:"total_tokens"`
	GrossQuota             int64   `json:"gross_quota"`
	RefundQuota            int64   `json:"refund_quota"`
	NetQuota               int64   `json:"net_quota"`
	AverageUseTimeSeconds  float64 `json:"average_use_time_seconds"`
	StreamRequests         int64   `json:"stream_requests"`
	LatestRequestTimestamp int64   `json:"latest_request_timestamp"`
}

type BillingStatementSummary struct {
	Requests              int64   `json:"requests"`
	PromptTokens          int64   `json:"prompt_tokens"`
	CompletionTokens      int64   `json:"completion_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	GrossQuota            int64   `json:"gross_quota"`
	RefundQuota           int64   `json:"refund_quota"`
	NetQuota              int64   `json:"net_quota"`
	AverageRPM            float64 `json:"average_rpm"`
	AverageTPM            float64 `json:"average_tpm"`
	AverageUseTimeSeconds float64 `json:"average_use_time_seconds"`
	StreamRatio           float64 `json:"stream_ratio"`
}

type billingStatementRow struct {
	TokenId                int
	TokenName              string
	ModelName              string
	Requests               int64
	PromptTokens           int64
	CompletionTokens       int64
	GrossQuota             int64
	RefundQuota            int64
	AverageUseTimeSeconds  float64
	StreamRequests         int64
	LatestRequestTimestamp int64
}

func GetUserBillingStatement(
	userId int,
	startTimestamp int64,
	endTimestamp int64,
	tokenId int,
	modelName string,
) ([]BillingStatementItem, BillingStatementSummary, error) {
	var rows []billingStatementRow
	// Async settlement adjustments carry task_id and must affect tokens and
	// quota, but they are not additional model calls.
	const taskSettlementPattern = `%"task_id"%`
	query := LOG_DB.Model(&Log{}).
		Select(`
			token_id,
			token_name,
			model_name,
			COALESCE(SUM(CASE WHEN type = ? AND COALESCE(other, '') NOT LIKE ? THEN 1 ELSE 0 END), 0) AS requests,
			COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) AS completion_tokens,
			COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS gross_quota,
			COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS refund_quota,
			COALESCE(AVG(CASE WHEN type = ? AND COALESCE(other, '') NOT LIKE ? THEN use_time ELSE NULL END), 0) AS average_use_time_seconds,
			COALESCE(SUM(CASE WHEN type = ? AND COALESCE(other, '') NOT LIKE ? AND is_stream THEN 1 ELSE 0 END), 0) AS stream_requests,
			MAX(created_at) AS latest_request_timestamp
		`,
			LogTypeConsume, taskSettlementPattern,
			LogTypeConsume,
			LogTypeRefund,
			LogTypeConsume, taskSettlementPattern,
			LogTypeConsume, taskSettlementPattern,
		).
		Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at <= ?",
			userId, []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp)
	if tokenId > 0 {
		query = query.Where("token_id = ?", tokenId)
	}
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	err := query.
		Group("token_id, token_name, model_name").
		Scan(&rows).Error
	if err != nil {
		return nil, BillingStatementSummary{}, err
	}

	type itemKey struct {
		tokenId   int
		modelName string
	}
	itemsByKey := make(map[itemKey]*BillingStatementItem, len(rows))
	for _, row := range rows {
		key := itemKey{tokenId: row.TokenId, modelName: row.ModelName}
		item, ok := itemsByKey[key]
		if !ok {
			item = &BillingStatementItem{
				TokenId:   row.TokenId,
				TokenName: row.TokenName,
				ModelName: row.ModelName,
			}
			itemsByKey[key] = item
		}
		if row.TokenName != "" && row.LatestRequestTimestamp >= item.LatestRequestTimestamp {
			item.TokenName = row.TokenName
			item.LatestRequestTimestamp = row.LatestRequestTimestamp
		}

		requests := max(row.Requests, int64(0))
		promptTokens := max(row.PromptTokens, int64(0))
		completionTokens := max(row.CompletionTokens, int64(0))
		grossQuota := max(row.GrossQuota, int64(0))
		refundQuota := max(row.RefundQuota, int64(0))
		streamRequests := max(row.StreamRequests, int64(0))
		if streamRequests > requests {
			streamRequests = requests
		}

		totalUseTime := max(row.AverageUseTimeSeconds, 0) * float64(requests)
		existingUseTime := item.AverageUseTimeSeconds * float64(item.Requests)
		item.Requests += requests
		item.PromptTokens += promptTokens
		item.CompletionTokens += completionTokens
		item.TotalTokens = item.PromptTokens + item.CompletionTokens
		item.GrossQuota += grossQuota
		item.RefundQuota += refundQuota
		item.NetQuota = max(item.GrossQuota-item.RefundQuota, int64(0))
		item.StreamRequests += streamRequests
		if item.Requests > 0 {
			item.AverageUseTimeSeconds = (existingUseTime + totalUseTime) / float64(item.Requests)
		}
	}

	items := make([]BillingStatementItem, 0, len(itemsByKey))
	var summary BillingStatementSummary
	var totalUseTime float64
	var streamRequests int64
	for _, item := range itemsByKey {
		items = append(items, *item)
		summary.Requests += item.Requests
		summary.PromptTokens += item.PromptTokens
		summary.CompletionTokens += item.CompletionTokens
		summary.GrossQuota += item.GrossQuota
		summary.RefundQuota += item.RefundQuota
		summary.NetQuota += item.NetQuota
		totalUseTime += item.AverageUseTimeSeconds * float64(item.Requests)
		streamRequests += item.StreamRequests
	}
	summary.TotalTokens = summary.PromptTokens + summary.CompletionTokens
	if summary.Requests > 0 {
		summary.AverageUseTimeSeconds = totalUseTime / float64(summary.Requests)
		summary.StreamRatio = float64(streamRequests) / float64(summary.Requests)
	}
	periodMinutes := float64(endTimestamp-startTimestamp) / 60
	if periodMinutes > 0 {
		summary.AverageRPM = float64(summary.Requests) / periodMinutes
		summary.AverageTPM = float64(summary.TotalTokens) / periodMinutes
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].NetQuota != items[j].NetQuota {
			return items[i].NetQuota > items[j].NetQuota
		}
		if items[i].TotalTokens != items[j].TotalTokens {
			return items[i].TotalTokens > items[j].TotalTokens
		}
		if items[i].ModelName != items[j].ModelName {
			return items[i].ModelName < items[j].ModelName
		}
		return items[i].TokenId < items[j].TokenId
	})
	return items, summary, nil
}
