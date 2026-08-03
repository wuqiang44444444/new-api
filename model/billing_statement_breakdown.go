package model

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_statement_setting"
)

type BillingStatementCacheBreakdown struct {
	HitRequests          int64   `json:"hit_requests"`
	WriteRequests        int64   `json:"write_requests"`
	DenominatorRequests  int64   `json:"denominator_requests"`
	DenominatorScope     string  `json:"denominator_scope"`
	ReadTokens           int64   `json:"read_tokens"`
	WriteTokens          int64   `json:"write_tokens"`
	WriteTokens5m        int64   `json:"write_tokens_5m"`
	WriteTokens1h        int64   `json:"write_tokens_1h"`
	HitRequestGrossQuota int64   `json:"hit_request_gross_quota"`
	HitRequestRatio      float64 `json:"hit_request_ratio"`
}

type BillingStatementContextBreakdown struct {
	ThresholdTokens        int64   `json:"threshold_tokens,omitempty"`
	ThresholdSource        string  `json:"threshold_source"`
	ClassifiedRequests     int64   `json:"classified_requests"`
	UnclassifiedRequests   int64   `json:"unclassified_requests"`
	ShortRequests          int64   `json:"short_requests"`
	LongRequests           int64   `json:"long_requests"`
	ShortInputTokens       int64   `json:"short_input_tokens"`
	LongInputTokens        int64   `json:"long_input_tokens"`
	ShortGrossQuota        int64   `json:"short_gross_quota"`
	LongGrossQuota         int64   `json:"long_gross_quota"`
	ClassificationCoverage float64 `json:"classification_coverage"`
}

type BillingStatementModeBreakdown struct {
	TieredRequests   int64 `json:"tiered_requests"`
	TieredGrossQuota int64 `json:"tiered_gross_quota"`
}

type BillingStatementBreakdownDataQuality struct {
	UnavailableRequests int64 `json:"unavailable_requests"`
}

type BillingStatementBreakdownItem struct {
	TokenId                    int                                   `json:"token_id"`
	TokenName                  string                                `json:"token_name"`
	ModelName                  string                                `json:"model_name"`
	Requests                   int64                                 `json:"requests"`
	GrossQuota                 int64                                 `json:"gross_quota"`
	UnallocatedAdjustmentQuota int64                                 `json:"unallocated_adjustment_quota,omitempty"`
	Cache                      *BillingStatementCacheBreakdown       `json:"cache,omitempty"`
	Context                    *BillingStatementContextBreakdown     `json:"context,omitempty"`
	BillingMode                *BillingStatementModeBreakdown        `json:"billing_mode,omitempty"`
	DataQuality                *BillingStatementBreakdownDataQuality `json:"data_quality,omitempty"`
}

type BillingStatementBreakdownSummary struct {
	Requests                   int64                                 `json:"requests"`
	GrossQuota                 int64                                 `json:"gross_quota"`
	UnallocatedAdjustmentQuota int64                                 `json:"unallocated_adjustment_quota,omitempty"`
	Cache                      *BillingStatementCacheBreakdown       `json:"cache,omitempty"`
	Context                    *BillingStatementContextBreakdown     `json:"context,omitempty"`
	BillingMode                *BillingStatementModeBreakdown        `json:"billing_mode,omitempty"`
	DataQuality                *BillingStatementBreakdownDataQuality `json:"data_quality,omitempty"`
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

const (
	billingStatementCacheDenominatorAllSettledRequests = "all_settled_requests"
	billingStatementContextThresholdCurrentConfig      = "current_model_config"
)

func GetUserBillingStatementBreakdown(
	userId int,
	startTimestamp int64,
	endTimestamp int64,
	tokenId int,
	modelName string,
) ([]BillingStatementBreakdownItem, BillingStatementBreakdownSummary, error) {
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
			"user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?",
			userId,
			LogTypeConsume,
			startTimestamp,
			endTimestamp,
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
		addBillingStatementValue(&accumulator.item.GrossQuota, quota)
		addBillingStatementValue(&summary.GrossQuota, quota)
		if strings.Contains(log.Other, `"task_id"`) {
			addBillingStatementValue(&accumulator.item.UnallocatedAdjustmentQuota, quota)
			addBillingStatementValue(&summary.UnallocatedAdjustmentQuota, quota)
			continue
		}
		addBillingStatementValue(&accumulator.item.Requests, 1)
		addBillingStatementValue(&summary.Requests, 1)

		var other map[string]json.RawMessage
		if log.Other != "" {
			if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
				if accumulator.item.DataQuality == nil {
					accumulator.item.DataQuality = &BillingStatementBreakdownDataQuality{}
				}
				if summary.DataQuality == nil {
					summary.DataQuality = &BillingStatementBreakdownDataQuality{}
				}
				addBillingStatementValue(&accumulator.item.DataQuality.UnavailableRequests, 1)
				addBillingStatementValue(&summary.DataQuality.UnavailableRequests, 1)
				continue
			}
		}
		cacheReadTokens, _ := billingBreakdownNonNegativeInt(other["cache_tokens"])
		cacheWriteTokens := normalizedBillingBreakdownCacheWriteTokens(other)
		if cacheReadTokens > 0 || cacheWriteTokens.total > 0 {
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
			if accumulator.item.Context == nil {
				accumulator.item.Context = &BillingStatementContextBreakdown{
					ThresholdTokens: threshold,
					ThresholdSource: billingStatementContextThresholdCurrentConfig,
				}
			}
			if summary.Context == nil {
				summary.Context = &BillingStatementContextBreakdown{
					ThresholdSource: billingStatementContextThresholdCurrentConfig,
				}
			}
			if inputTokens, classified := billingBreakdownInputTokens(log, other, cacheReadTokens, cacheWriteTokens.total); classified {
				accumulateBillingStatementContext(accumulator.item.Context, inputTokens, threshold, quota)
				accumulateBillingStatementContext(summary.Context, inputTokens, threshold, quota)
			} else {
				addBillingStatementValue(&accumulator.item.Context.UnclassifiedRequests, 1)
				addBillingStatementValue(&summary.Context.UnclassifiedRequests, 1)
			}
		}

		if billingBreakdownString(other["billing_mode"]) == "tiered_expr" {
			if accumulator.item.BillingMode == nil {
				accumulator.item.BillingMode = &BillingStatementModeBreakdown{}
			}
			if summary.BillingMode == nil {
				summary.BillingMode = &BillingStatementModeBreakdown{}
			}
			addBillingStatementValue(&accumulator.item.BillingMode.TieredRequests, 1)
			addBillingStatementValue(&accumulator.item.BillingMode.TieredGrossQuota, quota)
			addBillingStatementValue(&summary.BillingMode.TieredRequests, 1)
			addBillingStatementValue(&summary.BillingMode.TieredGrossQuota, quota)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, BillingStatementBreakdownSummary{}, err
	}

	items := make([]BillingStatementBreakdownItem, 0, len(itemsByKey))
	for _, accumulator := range itemsByKey {
		if accumulator.item.Cache != nil {
			finalizeBillingStatementCache(accumulator.item.Cache, accumulator.item.Requests)
		}
		if accumulator.item.Context != nil {
			finalizeBillingStatementContext(accumulator.item.Context)
		}
		items = append(items, accumulator.item)
	}
	if summary.Cache != nil {
		finalizeBillingStatementCache(summary.Cache, summary.Requests)
	}
	if summary.Context != nil {
		finalizeBillingStatementContext(summary.Context)
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

type billingStatementCacheWriteTokens struct {
	total       int64
	fiveMinutes int64
	oneHour     int64
}

func normalizedBillingBreakdownCacheWriteTokens(other map[string]json.RawMessage) billingStatementCacheWriteTokens {
	fiveMinutes, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens_5m"])
	oneHour, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens_1h"])
	writeTokens := billingStatementCacheWriteTokens{
		fiveMinutes: fiveMinutes,
		oneHour:     oneHour,
	}
	if tokens, ok := billingBreakdownNonNegativeInt(other["cache_write_tokens"]); ok {
		writeTokens.total = tokens
		return writeTokens
	}
	total, _ := billingBreakdownNonNegativeInt(other["cache_creation_tokens"])
	split := fiveMinutes
	addBillingStatementValue(&split, oneHour)
	writeTokens.total = max(total, split)
	return writeTokens
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
		addBillingStatementValue(&total, cacheReadTokens)
		addBillingStatementValue(&total, cacheWriteTokens)
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
	writeTokens billingStatementCacheWriteTokens,
	quota int64,
) {
	if readTokens > 0 {
		addBillingStatementValue(&target.HitRequests, 1)
		addBillingStatementValue(&target.ReadTokens, readTokens)
		addBillingStatementValue(&target.HitRequestGrossQuota, quota)
	}
	if writeTokens.total > 0 {
		addBillingStatementValue(&target.WriteRequests, 1)
		addBillingStatementValue(&target.WriteTokens, writeTokens.total)
		addBillingStatementValue(&target.WriteTokens5m, writeTokens.fiveMinutes)
		addBillingStatementValue(&target.WriteTokens1h, writeTokens.oneHour)
	}
}

func finalizeBillingStatementCache(target *BillingStatementCacheBreakdown, requests int64) {
	target.DenominatorRequests = requests
	target.DenominatorScope = billingStatementCacheDenominatorAllSettledRequests
	target.HitRequestRatio = billingBreakdownRatio(target.HitRequests, requests)
}

func accumulateBillingStatementContext(
	target *BillingStatementContextBreakdown,
	inputTokens int64,
	threshold int64,
	quota int64,
) {
	addBillingStatementValue(&target.ClassifiedRequests, 1)
	if inputTokens > threshold {
		addBillingStatementValue(&target.LongRequests, 1)
		addBillingStatementValue(&target.LongInputTokens, inputTokens)
		addBillingStatementValue(&target.LongGrossQuota, quota)
		return
	}
	addBillingStatementValue(&target.ShortRequests, 1)
	addBillingStatementValue(&target.ShortInputTokens, inputTokens)
	addBillingStatementValue(&target.ShortGrossQuota, quota)
}

func finalizeBillingStatementContext(target *BillingStatementContextBreakdown) {
	denominator := target.ClassifiedRequests
	addBillingStatementValue(&denominator, target.UnclassifiedRequests)
	target.ClassificationCoverage = billingBreakdownRatio(target.ClassifiedRequests, denominator)
}

func billingBreakdownRatio(numerator int64, denominator int64) float64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
