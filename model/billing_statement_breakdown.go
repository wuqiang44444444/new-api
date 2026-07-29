package model

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_statement_setting"
)

type BillingStatementCacheBreakdown struct {
	HitRequests          int64   `json:"hit_requests"`
	WriteRequests        int64   `json:"write_requests"`
	ReadTokens           int64   `json:"read_tokens"`
	WriteTokens          int64   `json:"write_tokens"`
	HitRequestGrossQuota int64   `json:"hit_request_gross_quota"`
	HitRequestRatio      float64 `json:"hit_request_ratio"`
}

type BillingStatementContextBreakdown struct {
	ThresholdTokens    int64 `json:"threshold_tokens,omitempty"`
	ClassifiedRequests int64 `json:"classified_requests"`
	ShortRequests      int64 `json:"short_requests"`
	LongRequests       int64 `json:"long_requests"`
	ShortGrossQuota    int64 `json:"short_gross_quota"`
	LongGrossQuota     int64 `json:"long_gross_quota"`
}

type BillingStatementModeBreakdown struct {
	TieredRequests   int64 `json:"tiered_requests"`
	TieredGrossQuota int64 `json:"tiered_gross_quota"`
}

type BillingStatementBreakdownItem struct {
	TokenId     int                               `json:"token_id"`
	TokenName   string                            `json:"token_name"`
	ModelName   string                            `json:"model_name"`
	Requests    int64                             `json:"requests"`
	GrossQuota  int64                             `json:"gross_quota"`
	Cache       *BillingStatementCacheBreakdown   `json:"cache,omitempty"`
	Context     *BillingStatementContextBreakdown `json:"context,omitempty"`
	BillingMode *BillingStatementModeBreakdown    `json:"billing_mode,omitempty"`
}

type BillingStatementBreakdownSummary struct {
	Requests    int64                             `json:"requests"`
	GrossQuota  int64                             `json:"gross_quota"`
	Cache       *BillingStatementCacheBreakdown   `json:"cache,omitempty"`
	Context     *BillingStatementContextBreakdown `json:"context,omitempty"`
	BillingMode *BillingStatementModeBreakdown    `json:"billing_mode,omitempty"`
}

type billingStatementBreakdownLog struct {
	TokenId      int
	TokenName    string
	ModelName    string
	CreatedAt    int64
	PromptTokens int
	Quota        int
	Other        string
}

type billingStatementBreakdownItemAccumulator struct {
	item                   BillingStatementBreakdownItem
	latestRequestTimestamp int64
}

func GetUserBillingStatementBreakdown(
	userId int,
	startTimestamp int64,
	endTimestamp int64,
	tokenId int,
	modelName string,
) ([]BillingStatementBreakdownItem, BillingStatementBreakdownSummary, error) {
	const taskSettlementPattern = `%"task_id"%`
	query := LOG_DB.Model(&Log{}).
		Select(`
			token_id,
			COALESCE(token_name, '') AS token_name,
			COALESCE(model_name, '') AS model_name,
			created_at,
			prompt_tokens,
			quota,
			COALESCE(other, '') AS other
		`).
		Where(
			"user_id = ? AND type = ? AND created_at >= ? AND created_at <= ? AND COALESCE(other, '') NOT LIKE ?",
			userId,
			LogTypeConsume,
			startTimestamp,
			endTimestamp,
			taskSettlementPattern,
		)
	if tokenId > 0 {
		query = query.Where("token_id = ?", tokenId)
	}
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}

	rows, err := query.Rows()
	if err != nil {
		return nil, BillingStatementBreakdownSummary{}, err
	}
	defer rows.Close()

	type itemKey struct {
		tokenId   int
		modelName string
	}
	itemsByKey := make(map[itemKey]*billingStatementBreakdownItemAccumulator)
	var summary BillingStatementBreakdownSummary

	for rows.Next() {
		var log billingStatementBreakdownLog
		if err = rows.Scan(
			&log.TokenId,
			&log.TokenName,
			&log.ModelName,
			&log.CreatedAt,
			&log.PromptTokens,
			&log.Quota,
			&log.Other,
		); err != nil {
			return nil, BillingStatementBreakdownSummary{}, err
		}

		key := itemKey{tokenId: log.TokenId, modelName: log.ModelName}
		accumulator, ok := itemsByKey[key]
		if !ok {
			accumulator = &billingStatementBreakdownItemAccumulator{
				item: BillingStatementBreakdownItem{
					TokenId:   log.TokenId,
					TokenName: log.TokenName,
					ModelName: log.ModelName,
				},
			}
			itemsByKey[key] = accumulator
		}
		if log.TokenName != "" && log.CreatedAt >= accumulator.latestRequestTimestamp {
			accumulator.item.TokenName = log.TokenName
			accumulator.latestRequestTimestamp = log.CreatedAt
		}

		quota := max(int64(log.Quota), int64(0))
		addBillingBreakdownValue(&accumulator.item.Requests, 1)
		addBillingBreakdownValue(&accumulator.item.GrossQuota, quota)
		addBillingBreakdownValue(&summary.Requests, 1)
		addBillingBreakdownValue(&summary.GrossQuota, quota)

		var other map[string]json.RawMessage
		if log.Other != "" {
			_ = common.UnmarshalJsonStr(log.Other, &other)
		}
		cacheReadTokens, _ := billingBreakdownNonNegativeInt(other["cache_tokens"])
		cacheWriteTokens := normalizedBillingBreakdownCacheWriteTokens(other)
		if cacheReadTokens > 0 || cacheWriteTokens > 0 {
			if accumulator.item.Cache == nil {
				accumulator.item.Cache = &BillingStatementCacheBreakdown{}
			}
			if summary.Cache == nil {
				summary.Cache = &BillingStatementCacheBreakdown{}
			}
			accumulateBillingStatementCache(accumulator.item.Cache, cacheReadTokens, cacheWriteTokens, quota)
			accumulateBillingStatementCache(summary.Cache, cacheReadTokens, cacheWriteTokens, quota)
		}

		if threshold, configured := billing_statement_setting.GetContextThreshold(log.ModelName); configured {
			if inputTokens, classified := billingBreakdownInputTokens(log, other, cacheReadTokens, cacheWriteTokens); classified {
				if accumulator.item.Context == nil {
					accumulator.item.Context = &BillingStatementContextBreakdown{ThresholdTokens: threshold}
				}
				if summary.Context == nil {
					summary.Context = &BillingStatementContextBreakdown{}
				}
				accumulateBillingStatementContext(accumulator.item.Context, inputTokens, threshold, quota)
				accumulateBillingStatementContext(summary.Context, inputTokens, threshold, quota)
			}
		}

		if billingBreakdownString(other["billing_mode"]) == "tiered_expr" {
			if accumulator.item.BillingMode == nil {
				accumulator.item.BillingMode = &BillingStatementModeBreakdown{}
			}
			if summary.BillingMode == nil {
				summary.BillingMode = &BillingStatementModeBreakdown{}
			}
			addBillingBreakdownValue(&accumulator.item.BillingMode.TieredRequests, 1)
			addBillingBreakdownValue(&accumulator.item.BillingMode.TieredGrossQuota, quota)
			addBillingBreakdownValue(&summary.BillingMode.TieredRequests, 1)
			addBillingBreakdownValue(&summary.BillingMode.TieredGrossQuota, quota)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, BillingStatementBreakdownSummary{}, err
	}

	items := make([]BillingStatementBreakdownItem, 0, len(itemsByKey))
	for _, accumulator := range itemsByKey {
		if accumulator.item.Cache != nil {
			accumulator.item.Cache.HitRequestRatio = billingBreakdownRatio(
				accumulator.item.Cache.HitRequests,
				accumulator.item.Requests,
			)
		}
		items = append(items, accumulator.item)
	}
	if summary.Cache != nil {
		summary.Cache.HitRequestRatio = billingBreakdownRatio(summary.Cache.HitRequests, summary.Requests)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].GrossQuota != items[j].GrossQuota {
			return items[i].GrossQuota > items[j].GrossQuota
		}
		if items[i].ModelName != items[j].ModelName {
			return items[i].ModelName < items[j].ModelName
		}
		return items[i].TokenId < items[j].TokenId
	})
	return items, summary, nil
}

func normalizedBillingBreakdownCacheWriteTokens(other map[string]json.RawMessage) int64 {
	if tokens, ok := billingBreakdownNonNegativeInt(other["cache_write_tokens"]); ok {
		return tokens
	}
	total, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens"])
	fiveMinutes, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens_5m"])
	oneHour, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens_1h"])
	split := fiveMinutes
	addBillingBreakdownValue(&split, oneHour)
	return max(total, split)
}

func billingBreakdownInputTokens(
	log billingStatementBreakdownLog,
	other map[string]json.RawMessage,
	cacheReadTokens int64,
	cacheWriteTokens int64,
) (int64, bool) {
	if tokens, ok := billingBreakdownNonNegativeInt(other["input_tokens_total"]); ok && tokens > 0 {
		return tokens, true
	}

	usageSemantic := strings.ToLower(billingBreakdownString(other["usage_semantic"]))
	promptTokens := max(int64(log.PromptTokens), int64(0))
	switch usageSemantic {
	case "anthropic":
		total := promptTokens
		addBillingBreakdownValue(&total, cacheReadTokens)
		addBillingBreakdownValue(&total, cacheWriteTokens)
		return total, total > 0
	case "openai", "gemini":
		return promptTokens, promptTokens > 0
	default:
		return 0, false
	}
}

func billingBreakdownNonNegativeInt(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value int64
	if err := common.Unmarshal(raw, &value); err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

func billingBreakdownString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func accumulateBillingStatementCache(
	target *BillingStatementCacheBreakdown,
	readTokens int64,
	writeTokens int64,
	quota int64,
) {
	if readTokens > 0 {
		addBillingBreakdownValue(&target.HitRequests, 1)
		addBillingBreakdownValue(&target.ReadTokens, readTokens)
		addBillingBreakdownValue(&target.HitRequestGrossQuota, quota)
	}
	if writeTokens > 0 {
		addBillingBreakdownValue(&target.WriteRequests, 1)
		addBillingBreakdownValue(&target.WriteTokens, writeTokens)
	}
}

func accumulateBillingStatementContext(
	target *BillingStatementContextBreakdown,
	inputTokens int64,
	threshold int64,
	quota int64,
) {
	addBillingBreakdownValue(&target.ClassifiedRequests, 1)
	if inputTokens > threshold {
		addBillingBreakdownValue(&target.LongRequests, 1)
		addBillingBreakdownValue(&target.LongGrossQuota, quota)
		return
	}
	addBillingBreakdownValue(&target.ShortRequests, 1)
	addBillingBreakdownValue(&target.ShortGrossQuota, quota)
}

func addBillingBreakdownValue(target *int64, value int64) {
	if value <= 0 {
		return
	}
	if *target > math.MaxInt64-value {
		*target = math.MaxInt64
		return
	}
	*target += value
}

func billingBreakdownRatio(numerator int64, denominator int64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
