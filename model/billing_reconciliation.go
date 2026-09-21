package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	BillingReconciliationModeToken     = "token"
	BillingReconciliationModePerCall   = "per_call"
	BillingReconciliationModePerSecond = "per_second"
	BillingReconciliationModeUnknown   = "unknown"
)

var ErrBillingReconciliationVersionConflict = errors.New("billing reconciliation version conflict")

type ProviderBillingDiscount struct {
	Id               int64           `json:"id"`
	PeriodStart      int64           `json:"period_start" gorm:"bigint;not null;uniqueIndex:idx_provider_billing_discount_key,priority:1"`
	ChannelId        int             `json:"channel_id" gorm:"not null;uniqueIndex:idx_provider_billing_discount_key,priority:2;index"`
	ProviderModel    string          `json:"provider_model" gorm:"type:varchar(255);not null;uniqueIndex:idx_provider_billing_discount_key,priority:3"`
	BillingMode      string          `json:"billing_mode" gorm:"type:varchar(32);not null;uniqueIndex:idx_provider_billing_discount_key,priority:4"`
	Discount         decimal.Decimal `json:"discount" gorm:"type:decimal(12,8);not null"`
	CopiedFromPeriod int64           `json:"copied_from_period,omitempty" gorm:"bigint;not null"`
	Version          int64           `json:"version" gorm:"bigint;not null"`
	Reason           string          `json:"reason" gorm:"type:varchar(255);not null"`
	CreatedBy        int             `json:"created_by" gorm:"not null"`
	UpdatedBy        int             `json:"updated_by" gorm:"not null"`
	CreatedAt        int64           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64           `json:"updated_at" gorm:"autoUpdateTime"`
}

// ProviderBillingAudit is append-only. Corrections are represented by a new
// versioned write and a new audit row; no update/delete entry point is exposed.
type ProviderBillingAudit struct {
	Id         int64  `json:"id"`
	EntityType string `json:"entity_type" gorm:"type:varchar(32);not null;index:idx_provider_billing_audit_entity,priority:1"`
	EntityHash string `json:"entity_hash" gorm:"type:char(64);not null;index:idx_provider_billing_audit_entity,priority:2"`
	EntityKey  string `json:"entity_key" gorm:"type:text;not null"`
	Action     string `json:"action" gorm:"type:varchar(32);not null"`
	Before     string `json:"before" gorm:"type:text;not null"`
	After      string `json:"after" gorm:"type:text;not null"`
	Reason     string `json:"reason" gorm:"type:varchar(255);not null"`
	OperatorId int    `json:"operator_id" gorm:"not null;index"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func migrateBillingReconciliationDB() error {
	if err := DB.AutoMigrate(
		&ProviderBillingDiscount{},
		&ProviderBillingAudit{},
		&ProviderChannelBillingDiscount{},
		&ProviderURLGroupDisplayName{},
	); err != nil {
		return err
	}
	return migrateBillingStatementTaskIndex(DB)
}

type BillingReconciliationDataQuality struct {
	TestRecordedOriginalRows int64 `json:"test_recorded_original_rows,omitempty"`
	TestRecomputedRows       int64 `json:"test_recomputed_rows,omitempty"`
	// Exclusive reasons partition UsageWithoutAmountRows; never add them to that total.
	TestAmountPendingReasons       map[string]int64         `json:"test_amount_pending_reasons,omitempty"`
	RecoveredBillingSecondsRows    int64                    `json:"recovered_billing_seconds_rows,omitempty"`
	RefundedTaskHoldRows           int64                    `json:"refunded_task_hold_rows,omitempty"`
	SecondsTaskLinkMissingRows     int64                    `json:"seconds_task_link_missing_rows,omitempty"`
	LegacyTestCacheReadRows        int64                    `json:"legacy_test_cache_read_rows,omitempty"`
	LegacyTestCacheWriteRows       int64                    `json:"legacy_test_cache_write_rows,omitempty"`
	EvidenceCoverage               *BillingEvidenceCoverage `json:"evidence_coverage,omitempty"`
	CacheReadUnreportedRequests    int64                    `json:"cache_read_unreported_requests,omitempty"`
	CacheWriteUnreportedRequests   int64                    `json:"cache_write_unreported_requests,omitempty"`
	CacheReadUnavailableRequests   int64                    `json:"cache_read_unavailable_requests,omitempty"`
	SecondsUnavailableRows         int64                    `json:"seconds_unavailable_rows,omitempty"`
	InputTokensUnavailableRequests int64                    `json:"input_tokens_unavailable_requests,omitempty"`
	CacheWriteUnavailableRequests  int64                    `json:"cache_write_unavailable_requests,omitempty"`
	Status                         string                   `json:"status"`
	UnavailableRequests            int64                    `json:"unavailable_requests,omitempty"`
	UnknownBillingModeRequests     int64                    `json:"unknown_billing_mode_requests,omitempty"`
	ProviderModelFallbackRows      int64                    `json:"provider_model_fallback_rows,omitempty"`
	MissingHistoricalPriceRows     int64                    `json:"missing_historical_price_rows,omitempty"`
	AuxiliaryChargeRows            int64                    `json:"auxiliary_charge_rows,omitempty"`
	TestPricedRows                 int64                    `json:"test_priced_rows,omitempty"`
	SecondsValueMissingRows        int64                    `json:"seconds_value_missing_rows,omitempty"`
	// UsageWithoutAmountRows counts native channel test rows kept in the usage
	// view whose amount could not be confirmed from frozen pricing facts. Priced
	// tests are counted in TestPricedRows and enter the reference amount, so
	// this is no longer "all tests"; usage and amount coverage stay distinct.
	UsageWithoutAmountRows int64 `json:"usage_without_amount_rows,omitempty"`
}

type BillingReconciliationUsage struct {
	Requests         int64 `json:"requests"`
	BillableCalls    int64 `json:"billable_calls"`
	RefundedCalls    int64 `json:"refunded_calls"`
	InputTokens      int64 `json:"input_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	GrossQuota       int64 `json:"gross_quota"`
	RefundQuota      int64 `json:"refund_quota"`
	NetQuota         int64 `json:"net_quota"`
}

type BillingReconciliationModelSummary struct {
	DiscountQuota             *int64   `json:"discount_quota,omitempty"`
	EstimateReasons           []string `json:"estimate_reasons,omitempty"`
	originalQuota             decimal.Decimal
	ModelName                 string                            `json:"model_name"`
	BillingMode               string                            `json:"billing_mode"`
	Usage                     BillingReconciliationUsage        `json:"usage"`
	OriginalQuota             *int64                            `json:"original_quota,omitempty"`
	DiscountRatio             *float64                          `json:"discount_ratio,omitempty"`
	MultipleDiscounts         bool                              `json:"multiple_discounts,omitempty"`
	ContractDiscountRatio     *float64                          `json:"contract_discount_ratio,omitempty"`
	MultipleContractDiscounts bool                              `json:"multiple_contract_discounts,omitempty"`
	PriceVersions             int64                             `json:"price_versions"`
	DataQuality               *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter              BillingReconciliationDetailFilter `json:"detail_filter"`
}

type BillingReconciliationGroupSummary struct {
	EstimateReasons []string `json:"estimate_reasons,omitempty"`
	originalQuota   decimal.Decimal
	Id              int64                               `json:"id"`
	Name            string                              `json:"name"`
	Usage           BillingReconciliationUsage          `json:"usage"`
	OriginalQuota   *int64                              `json:"original_quota,omitempty"`
	DiscountQuota   *int64                              `json:"discount_quota,omitempty"`
	Models          []BillingReconciliationModelSummary `json:"models"`
	Deleted         bool                                `json:"deleted,omitempty"`
}

type BillingReconciliationDetailFilter struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	UserId         int    `json:"user_id,omitempty"`
	TokenId        int    `json:"token_id"`
	ChannelId      int    `json:"channel_id,omitempty"`
	ModelName      string `json:"model_name,omitempty"`
	BillingMode    string `json:"billing_mode,omitempty"`
}

type BillingCustomerStatement struct {
	EstimateReasons []string                            `json:"estimate_reasons,omitempty"`
	UserId          int                                 `json:"user_id"`
	Username        string                              `json:"username"`
	DisplayName     string                              `json:"display_name"`
	Deleted         bool                                `json:"deleted,omitempty"`
	Dimension       string                              `json:"dimension"`
	CurrentBalance  *int                                `json:"current_balance"`
	Summary         BillingReconciliationUsage          `json:"summary"`
	OriginalQuota   *int64                              `json:"original_quota,omitempty"`
	DiscountQuota   *int64                              `json:"discount_quota,omitempty"`
	Groups          []BillingReconciliationGroupSummary `json:"groups"`
	// DiscountCombinations is the plan-section-4 discount-combination
	// projection: the same settled logs, one row per frozen discount-fact
	// combination, reconcilable with the statement totals.
	DiscountCombinations []BillingDiscountCombination      `json:"discount_combinations,omitempty"`
	DataQuality          *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

type billingReconciliationLog struct {
	RequestId        string
	GroupName        string
	Content          string
	UserId           int
	TokenId          int
	TokenName        string
	ChannelId        int
	ModelName        string
	Type             int
	CreatedAt        int64
	PromptTokens     int
	CompletionTokens int
	Quota            int
	Other            string
}

type billingReconciliationModelAccumulator struct {
	model                     BillingReconciliationModelSummary
	discountSeen              bool
	discountRatio             float64
	contractDiscountSeen      bool
	hasContractDiscountFact   bool
	contractDiscountRatio     float64
	multipleContractDiscounts bool
	originalQuota             decimal.Decimal
	originalQuotaKnown        bool
	originalQuotaComplete     bool
	priceSnapshotMarkers      map[string]struct{}
}

func GetBillingCustomerStatement(
	ctx context.Context,
	userId int,
	startTimestamp int64,
	endTimestamp int64,
	dimension string,
	groupId int,
	modelName string,
	billingMode string,
	readPolicies ...BillingStatementReadPolicy,
) (BillingCustomerStatement, error) {
	statement := BillingCustomerStatement{
		UserId:    userId,
		Dimension: dimension,
		Groups:    make([]BillingReconciliationGroupSummary, 0),
	}
	if dimension != "api_key" && dimension != "channel" {
		return statement, errors.New("invalid billing dimension")
	}
	if billingMode != "" && billingMode != BillingReconciliationModeToken && billingMode != BillingReconciliationModePerCall && billingMode != BillingReconciliationModePerSecond && billingMode != BillingReconciliationModeUnknown {
		return statement, errors.New("invalid billing mode")
	}

	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Scopes(customerSettlementLogs).
		Select("user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(content, '') AS content, COALESCE(other, '') AS other, "+billingStatementGroupSelect()).
		Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at <= ?", userId, []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp)
	if groupId > 0 {
		if dimension == "api_key" {
			query = query.Where("token_id = ?", groupId)
		} else {
			query = query.Where("channel_id = ?", groupId)
		}
	}

	type groupKey struct {
		id int64
	}
	type modelKey struct {
		groupId int64
		model   string
		mode    string
	}
	groups := make(map[groupKey]*BillingReconciliationGroupSummary)
	models := make(map[modelKey]*billingReconciliationModelAccumulator)
	groupNames := make(map[int64]string)
	combinations := newBillingDiscountCombinationAccumulator()

	var policy BillingStatementReadPolicy
	if len(readPolicies) > 0 {
		policy = readPolicies[0]
	}
	err := scanBillingStatementFacts(ctx, query, policy, func(log billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
		log.ModelName = parsed.customerModel
		if modelName != "" && log.ModelName != modelName {
			return nil
		}
		if billingMode != "" && parsed.billingMode != billingMode {
			return nil
		}

		selectedGroupId := int64(log.TokenId)
		selectedGroupName := log.TokenName
		if dimension == "channel" {
			selectedGroupId = int64(log.ChannelId)
			selectedGroupName = ""
		}
		combinations.observe(log, selectedGroupId, parsed)
		gk := groupKey{id: selectedGroupId}
		group, ok := groups[gk]
		if !ok {
			group = &BillingReconciliationGroupSummary{
				Id:     selectedGroupId,
				Models: make([]BillingReconciliationModelSummary, 0),
			}
			groups[gk] = group
		}
		if selectedGroupName != "" {
			groupNames[selectedGroupId] = selectedGroupName
		}

		mk := modelKey{groupId: selectedGroupId, model: log.ModelName, mode: parsed.billingMode}
		accumulator, ok := models[mk]
		if !ok {
			accumulator = &billingReconciliationModelAccumulator{
				model: BillingReconciliationModelSummary{
					ModelName:   log.ModelName,
					BillingMode: parsed.billingMode,
					DetailFilter: BillingReconciliationDetailFilter{
						StartTimestamp: startTimestamp,
						EndTimestamp:   endTimestamp,
						UserId:         userId,
						ModelName:      log.ModelName,
						BillingMode:    parsed.billingMode,
					},
				},
				priceSnapshotMarkers:  make(map[string]struct{}),
				originalQuotaComplete: true,
			}
			if dimension == "api_key" {
				accumulator.model.DetailFilter.TokenId = log.TokenId
			} else {
				accumulator.model.DetailFilter.ChannelId = log.ChannelId
			}
			models[mk] = accumulator
		}
		accumulateBillingReconciliationLog(&accumulator.model.Usage, log, parsed)
		if parsed.inputTokensUnavailable {
			ensureBillingReconciliationQuality(&accumulator.model.DataQuality).InputTokensUnavailableRequests++
		}
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&accumulator.model.DataQuality).UnavailableRequests++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&accumulator.model.DataQuality).UnknownBillingModeRequests++
		}
		accumulateBillingReconciliationPrice(accumulator, log, parsed)
		if policy.MaxGroups > 0 && len(models) > policy.MaxGroups {
			return errors.New("statement aggregation group budget exceeded")
		}
		return nil
	})
	if err != nil {
		return statement, err
	}

	if dimension == "channel" {
		channelIds := make([]int, 0, len(groups))
		for key := range groups {
			if key.id > 0 {
				channelIds = append(channelIds, int(key.id))
			}
		}
		if len(channelIds) > 0 {
			var channels []struct {
				Id   int
				Name string
			}
			if err := DB.Model(&Channel{}).Select("id, name").Where("id IN ?", channelIds).Scan(&channels).Error; err != nil {
				return statement, err
			}
			for _, channel := range channels {
				groupNames[int64(channel.Id)] = channel.Name
			}
		}
	}

	for key, accumulator := range models {
		finalizeBillingReconciliationUsage(&accumulator.model.Usage)
		finalizeBillingReconciliationPrice(accumulator)
		finalizeBillingReconciliationQuality(&accumulator.model.DataQuality)
		accumulateBillingReconciliationQuality(&statement.DataQuality, accumulator.model.DataQuality)
		group := groups[groupKey{id: key.groupId}]
		group.Models = append(group.Models, accumulator.model)
		accumulateBillingReconciliationUsage(&group.Usage, accumulator.model.Usage)
	}
	for key, group := range groups {
		name, ok := groupNames[key.id]
		if !ok || strings.TrimSpace(name) == "" {
			group.Deleted = key.id > 0 && !ok
			if dimension == "api_key" {
				name = fmt.Sprintf("API Key #%d", key.id)
			} else {
				name = fmt.Sprintf("Channel #%d", key.id)
			}
		}
		group.Name = name
		finalizeBillingReconciliationUsage(&group.Usage)
		finalizeBillingReconciliationOriginalQuota(group)
		sort.Slice(group.Models, func(i, j int) bool {
			if group.Models[i].Usage.NetQuota != group.Models[j].Usage.NetQuota {
				return group.Models[i].Usage.NetQuota > group.Models[j].Usage.NetQuota
			}
			if group.Models[i].ModelName != group.Models[j].ModelName {
				return group.Models[i].ModelName < group.Models[j].ModelName
			}
			return group.Models[i].BillingMode < group.Models[j].BillingMode
		})
		statement.Groups = append(statement.Groups, *group)
		accumulateBillingReconciliationUsage(&statement.Summary, group.Usage)
	}
	finalizeBillingReconciliationUsage(&statement.Summary)
	finalizeBillingCustomerStatementOriginalQuota(&statement)
	finalizeBillingReconciliationQuality(&statement.DataQuality)
	statement.DiscountCombinations = combinations.finalize()
	sort.Slice(statement.Groups, func(i, j int) bool {
		if statement.Groups[i].Usage.NetQuota != statement.Groups[j].Usage.NetQuota {
			return statement.Groups[i].Usage.NetQuota > statement.Groups[j].Usage.NetQuota
		}
		return statement.Groups[i].Name < statement.Groups[j].Name
	})

	identity, err := GetBillingReconciliationUserById(userId)
	if err != nil {
		return statement, err
	}
	statement.Username = identity.Username
	statement.DisplayName = identity.DisplayName
	statement.Deleted = identity.Deleted
	statement.CurrentBalance = identity.CurrentBalance
	return statement, nil
}

func finalizeBillingReconciliationOriginalQuota(group *BillingReconciliationGroupSummary) {
	if group == nil {
		return
	}
	group.EstimateReasons = nil
	for _, item := range group.Models {
		group.EstimateReasons = mergeBillingEstimateReasons(group.EstimateReasons, item.EstimateReasons...)
	}
	originalQuota := decimal.Zero
	for _, item := range group.Models {
		if (item.Usage.GrossQuota > 0 || item.Usage.RefundQuota > 0) && item.OriginalQuota == nil {
			return
		}
		if item.OriginalQuota != nil {
			originalQuota = originalQuota.Add(item.originalQuota)
		}
	}
	group.originalQuota = originalQuota
	group.OriginalQuota = billingStatementOriginalQuota(originalQuota)
	if group.OriginalQuota == nil {
		group.EstimateReasons = mergeBillingEstimateReasons(group.EstimateReasons, BillingEstimateAmountOutOfRange)
		return
	}
	discountQuota := *group.OriginalQuota - group.Usage.NetQuota
	group.DiscountQuota = &discountQuota
}

func finalizeBillingCustomerStatementOriginalQuota(statement *BillingCustomerStatement) {
	if statement == nil {
		return
	}
	statement.EstimateReasons = nil
	for _, group := range statement.Groups {
		statement.EstimateReasons = mergeBillingEstimateReasons(statement.EstimateReasons, group.EstimateReasons...)
	}
	originalQuota := decimal.Zero
	for _, group := range statement.Groups {
		if (group.Usage.GrossQuota > 0 || group.Usage.RefundQuota > 0) && group.OriginalQuota == nil {
			return
		}
		if group.OriginalQuota != nil {
			originalQuota = originalQuota.Add(group.originalQuota)
		}
	}
	statement.OriginalQuota = billingStatementOriginalQuota(originalQuota)
	if statement.OriginalQuota == nil {
		statement.EstimateReasons = mergeBillingEstimateReasons(statement.EstimateReasons, BillingEstimateAmountOutOfRange)
		return
	}
	discountQuota := *statement.OriginalQuota - statement.Summary.NetQuota
	statement.DiscountQuota = &discountQuota
}

// upstreamTestPricingRecord is the pricing evidence a channel test persists at
// its own log boundary (other["test_pricing"]). New records carry the engine's
// pre-discount original directly; historical rows lack it and fall back to
// evidence-based classification.
type upstreamTestPricingRecord struct {
	Version       int    `json:"version"`
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	OriginalQuota *int64 `json:"original_quota"`
	ExprVersion   int    `json:"expr_version,omitempty"`
}

const (
	upstreamTestPricingModeRatio      = "ratio"
	upstreamTestPricingModeFixedPrice = "fixed_price"
	upstreamTestPricingModeTieredExpr = "tiered_expr"

	upstreamTestPricingStatusSettled   = "settled"
	upstreamTestPricingStatusEstimated = "estimated"
)

type parsedBillingReconciliationLog struct {
	customerModel           string
	refundTaskID            string
	inputTokens             int64
	recordedInputTokens     int64
	outputTokens            int64
	inputTokensUnavailable  bool
	cacheWriteUnavailable   bool
	billingMode             string
	isRequest               bool
	requestCount            int64
	isRefund                bool
	isChannelTest           bool
	hasExpression           bool
	taskID                  string
	isTask                  bool
	taskBillingEvent        string
	hasImageCount           bool
	modelRatio              *float64
	completionRatio         *float64
	modelPrice              *float64
	testPricing             *upstreamTestPricingRecord
	cacheReadTokens         int64
	cacheReadUnavailable    bool
	cacheReadEvidence       upstreamCacheEvidence
	cacheWriteEvidence      upstreamCacheEvidence
	cacheWrite              billingStatementCacheWriteTokens
	discountRatio           *float64
	groupName               string
	groupRatioSource        string
	contractDiscountRatio   *float64
	contractEvidence        bool
	contractApplicableKnown bool
	contractApplicable      bool
	contractId              float64
	contractVersion         float64
	contractName            string
	refundPreauthLogId      int64
	hasAuxiliaryCharge      bool
	priceMarker             string
	providerModel           string
	unavailable             bool
}

func parseBillingReconciliationLog(log billingReconciliationLog) parsedBillingReconciliationLog {
	test := isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content)
	parsed := parsedBillingReconciliationLog{isChannelTest: test, cacheWriteUnavailable: test, billingMode: BillingReconciliationModeUnknown, isRequest: log.Type == LogTypeConsume, isRefund: log.Type == LogTypeRefund}
	parsed.customerModel = log.ModelName
	parsed.groupName = log.GroupName
	parsed.inputTokens = max(int64(log.PromptTokens), 0)
	parsed.recordedInputTokens = parsed.inputTokens
	parsed.outputTokens = max(int64(log.CompletionTokens), 0)
	if strings.TrimSpace(log.Other) == "" {
		if log.PromptTokens > 0 || log.CompletionTokens > 0 {
			parsed.billingMode = BillingReconciliationModeToken
		}
		parsed.unavailable = true
		return parsed
	}
	var other map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
		parsed.unavailable = true
		return parsed
	}
	cacheReadTokens, cacheReadKnown := billingBreakdownNonNegativeInt(other["cache_tokens"])
	parsed.cacheReadTokens = cacheReadTokens
	path := strings.TrimSuffix(billingBreakdownString(other["request_path"]), "/")
	image := path == "/v1/images/generations" || path == "/v1/images/edits"
	parsed.cacheReadEvidence = upstreamCacheMeterEvidence(other, "cache_read_tokens_reported", cacheReadKnown)
	_, cacheReadInstrumented := other["cache_read_tokens_reported"]
	parsed.cacheReadUnavailable = (image || cacheReadInstrumented) && parsed.cacheReadEvidence != upstreamCacheRecorded
	parsed.cacheWrite = normalizedBillingBreakdownCacheWriteTokens(other)
	if !parsed.cacheWrite.known && (nativeUsageLogOmitsZeroCacheWrite(log, other) || historicalTestOmitsCacheWrite(log, other)) {
		parsed.cacheWrite.known = true
	}
	parsed.cacheWriteEvidence = upstreamCacheMeterEvidence(other, "cache_write_tokens_reported", parsed.cacheWrite.known)
	parsed.cacheWriteUnavailable = !parsed.cacheWrite.known && parsed.isChannelTest
	if raw := other["test_pricing"]; len(raw) > 0 && parsed.isChannelTest {
		var record upstreamTestPricingRecord
		// An invalid explicit record must not fall through to historical pricing.
		_ = common.Unmarshal(raw, &record)
		parsed.testPricing = &record
	}
	parsed.providerModel = billingBreakdownString(other["upstream_model_name"])
	isModelMapped, _ := billingReconciliationBool(other["is_model_mapped"])
	if parsed.providerModel == "" && !isModelMapped {
		// The relay sends the requested model name upstream unless model mapping
		// was explicitly applied. Legacy logs already persist that mapping marker,
		// so an unmarked row is an exact identity, not an unknown-model fallback.
		parsed.providerModel = strings.TrimSpace(log.ModelName)
	}

	snapshotRaw := other["statement_snapshot"]
	if len(snapshotRaw) == 0 {
		if adminRaw := other["admin_info"]; len(adminRaw) > 0 {
			var adminInfo map[string]json.RawMessage
			if common.Unmarshal(adminRaw, &adminInfo) == nil {
				snapshotRaw = adminInfo["statement_snapshot"]
			}
		}
	}
	if log.Type == LogTypeRefund {
		// 人工退款关联（质量方案阶段 B）：只读取显式的原预扣引用身份，
		// 不按时间、金额或模型猜测关联；引用有效性由退款事实层校验。
		if adminRaw := other["admin_info"]; len(adminRaw) > 0 {
			var adminInfo map[string]json.RawMessage
			if common.Unmarshal(adminRaw, &adminInfo) == nil {
				preauthId, _ := billingReconciliationFloat(adminInfo["original_preauth_log_id"])
				parsed.refundPreauthLogId = int64(preauthId)
			}
		}
	}
	var snapshot map[string]json.RawMessage
	if len(snapshotRaw) > 0 {
		if common.Unmarshal(snapshotRaw, &snapshot) == nil {
			snapshotMode := billingBreakdownString(snapshot["billing_mode"])
			if snapshotMode == BillingReconciliationModeToken || snapshotMode == BillingReconciliationModePerCall || snapshotMode == BillingReconciliationModePerSecond {
				parsed.billingMode = snapshotMode
			}
			if providerModel := billingBreakdownString(snapshot["provider_model"]); providerModel != "" {
				parsed.providerModel = providerModel
			}
			if customerModel := billingBreakdownString(snapshot["customer_model"]); customerModel != "" {
				parsed.customerModel = customerModel
			}
		}
	}
	requestPath := billingBreakdownString(other["request_path"])
	isTask, _ := billingReconciliationBool(other["is_task"])
	parsed.taskID = billingBreakdownString(other["task_id"])
	parsed.isTask = isTask
	parsed.taskBillingEvent = billingBreakdownString(other["task_billing_event"])
	parsed.hasImageCount = len(other["image_count"]) > 0
	if parsed.billingMode == BillingReconciliationModeUnknown && (isTask || billingBreakdownString(other["task_id"]) != "" || strings.Contains(requestPath, "/mj/")) {
		parsed.billingMode = BillingReconciliationModePerCall
	}
	if parsed.billingMode == BillingReconciliationModeUnknown && billingBreakdownString(other["billing_mode"]) == "tiered_expr" {
		parsed.billingMode = BillingReconciliationModeToken
	}
	modelPrice, modelPriceOk := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "model_price"))
	if parsed.billingMode == BillingReconciliationModeUnknown && modelPriceOk && modelPrice > 0 {
		parsed.billingMode = BillingReconciliationModePerCall
	}
	if modelPriceOk {
		parsed.modelPrice = &modelPrice
	}
	modelRatio, modelRatioOk := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "model_ratio"))
	if parsed.billingMode == BillingReconciliationModeUnknown && modelRatioOk && modelRatio > 0 {
		parsed.billingMode = BillingReconciliationModeToken
	}
	// 渠道测试原价还原需要记录发生时的倍率证据；只冻结数值本身，不读当前配置。
	if modelRatioOk {
		parsed.modelRatio = &modelRatio
	}
	if completionRatio, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "completion_ratio")); ok {
		parsed.completionRatio = &completionRatio
	}
	if parsed.billingMode == BillingReconciliationModeUnknown && (log.PromptTokens > 0 || log.CompletionTokens > 0 || parsed.cacheReadTokens > 0 || parsed.cacheWrite.total > 0) {
		parsed.billingMode = BillingReconciliationModeToken
	}
	if ratio, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "group_ratio")); ok && ratio > 0 {
		parsed.discountRatio = &ratio
		parsed.groupRatioSource = "group"
	}
	if ratio, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "user_group_ratio")); ok && ratio > 0 {
		parsed.discountRatio = &ratio
		parsed.groupRatioSource = "user_exclusive"
	}
	if ratio, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "contract_discount")); ok && ratio > 0 {
		parsed.contractDiscountRatio = &ratio
	}
	// 折扣组合的键事实（1.3）：合同适用状态、身份与版本只取发生时冻结值；
	// 未命中合同时显式记录 false；历史完全无合同记录时按合同因子 1 估算。
	parsed.contractEvidence = len(billingReconciliationSnapshotRaw(snapshot, other, "contract_discount")) > 0 || len(billingReconciliationSnapshotRaw(snapshot, other, "contract_applicable")) > 0
	if applicable, ok := billingReconciliationBool(billingReconciliationSnapshotRaw(snapshot, other, "contract_applicable")); ok {
		parsed.contractApplicableKnown = true
		parsed.contractApplicable = applicable
	}
	if parsed.contractApplicableKnown && !parsed.contractApplicable && parsed.contractDiscountRatio != nil {
		parsed.unavailable = true
	}
	parsed.contractId, _ = billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "contract_id"))
	parsed.contractVersion, _ = billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "contract_version"))
	parsed.contractName = billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "contract_name"))
	// Tool and operation surcharges are appended after the model charge and do
	// not carry a separate historical quota amount. Do not pretend the whole
	// settled quota can be reversed by a price ratio when such a surcharge is
	// present.
	parsed.hasAuxiliaryCharge = len(other["tool_surcharges"]) > 0 ||
		len(other["fee_quota"]) > 0 || len(other["image_generation_call_price"]) > 0 ||
		len(other["web_search_price"]) > 0 || len(other["file_search_price"]) > 0
	priceMarkers := []string{
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "model_price")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "model_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "completion_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "cache_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "group_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "contract_discount")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64")),
	}
	for _, marker := range priceMarkers {
		if marker != "" {
			parsed.priceMarker = strings.Join(priceMarkers, "|")
			break
		}
	}
	billingStatementTaskFacts(log, other, snapshot, &parsed)
	billingStatementBatchFacts(log, other, &parsed)
	applyUpstreamTaskCacheApplicability(log, other, snapshot, &parsed)
	if total, known := billingBreakdownInputTokens(billingStatementBreakdownLog{PromptTokens: log.PromptTokens}, other, parsed.cacheReadTokens, parsed.cacheWrite.total); known {
		parsed.inputTokens = total
	} else if parsed.cacheReadTokens > 0 || parsed.cacheWrite.total > 0 {
		// Cache-bearing rows without a frozen input semantic cannot be added
		// to normalized totals. Keep known money and cache usage independently.
		parsed.inputTokens = 0
		parsed.inputTokensUnavailable = true
	}
	parsed.cacheWriteUnavailable = parsed.cacheWriteUnavailable && parsed.billingMode != BillingReconciliationModePerCall
	return parsed
}

func billingReconciliationSnapshotRaw(snapshot map[string]json.RawMessage, other map[string]json.RawMessage, key string) json.RawMessage {
	if raw := snapshot[key]; len(raw) > 0 {
		return raw
	}
	return other[key]
}

func billingReconciliationFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var value float64
	if err := common.Unmarshal(raw, &value); err == nil {
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	var text string
	if err := common.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func billingReconciliationBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var value bool
	if err := common.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

func billingReconciliationRawMarker(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return string(raw)
}

func accumulateBillingReconciliationLog(target *BillingReconciliationUsage, log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	quota := max(int64(log.Quota), int64(0))
	if log.Type == LogTypeConsume {
		if parsed.isRequest {
			target.Requests += billingStatementRequestCount(parsed)
			if parsed.billingMode == BillingReconciliationModePerCall {
				target.BillableCalls++
			}
		}
		target.GrossQuota += quota
	} else if log.Type == LogTypeRefund {
		if parsed.isRefund && parsed.billingMode == BillingReconciliationModePerCall {
			target.RefundedCalls++
		}
		target.RefundQuota += quota
	}
	target.InputTokens += parsed.inputTokens
	target.OutputTokens += parsed.outputTokens
	target.CacheReadTokens += parsed.cacheReadTokens
	target.CacheWriteTokens += parsed.cacheWrite.total
}

func accumulateBillingReconciliationPrice(accumulator *billingReconciliationModelAccumulator, log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	if log.Type != LogTypeConsume && log.Type != LogTypeRefund {
		return
	}
	if parsed.priceMarker != "" {
		accumulator.priceSnapshotMarkers[parsed.priceMarker] = struct{}{}
	}
	quota := max(int64(log.Quota), int64(0))
	if quota == 0 {
		return
	}
	if reasons := billingStatementEstimateReasons(parsed); len(reasons) > 0 {
		accumulator.model.EstimateReasons = mergeBillingEstimateReasons(accumulator.model.EstimateReasons, reasons...)
		accumulator.originalQuotaComplete = false
		accumulateBillingEstimateReasonQuality(ensureBillingReconciliationQuality(&accumulator.model.DataQuality), reasons)
	} else {
		ratio := *parsed.discountRatio
		if !accumulator.discountSeen {
			accumulator.discountSeen = true
			accumulator.discountRatio = ratio
		} else if accumulator.discountRatio != ratio {
			accumulator.model.MultipleDiscounts = true
		}

		contractRatio := 1.0
		if parsed.contractDiscountRatio != nil && *parsed.contractDiscountRatio > 0 {
			contractRatio = *parsed.contractDiscountRatio
			accumulator.hasContractDiscountFact = true
		}
		if !accumulator.contractDiscountSeen {
			accumulator.contractDiscountSeen = true
			accumulator.contractDiscountRatio = contractRatio
		} else if accumulator.contractDiscountRatio != contractRatio {
			accumulator.multipleContractDiscounts = true
		}

		original := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(ratio)).Div(decimal.NewFromFloat(contractRatio))
		if log.Type == LogTypeRefund {
			original = original.Neg()
		}
		accumulator.originalQuota = accumulator.originalQuota.Add(original)
		accumulator.originalQuotaKnown = true
	}
}

// Billing statements aggregate signed refunds before rounding. These int64
// display totals are not individual wallet charges (which use int32 bounds).
func billingStatementOriginalQuota(amount decimal.Decimal) *int64 {
	if amount.Abs().Round(0).GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return nil
	}
	value := amount.Round(0).IntPart()
	return &value
}

func finalizeBillingReconciliationPrice(accumulator *billingReconciliationModelAccumulator) {
	accumulator.model.PriceVersions = int64(len(accumulator.priceSnapshotMarkers))
	if accumulator.originalQuotaComplete && (accumulator.originalQuotaKnown || (accumulator.model.Usage.GrossQuota == 0 && accumulator.model.Usage.RefundQuota == 0)) {
		accumulator.model.originalQuota = accumulator.originalQuota
		accumulator.model.OriginalQuota = billingStatementOriginalQuota(accumulator.originalQuota)
		if accumulator.model.OriginalQuota == nil {
			accumulator.model.EstimateReasons = mergeBillingEstimateReasons(accumulator.model.EstimateReasons, BillingEstimateAmountOutOfRange)
			accumulateBillingEstimateReasonQuality(ensureBillingReconciliationQuality(&accumulator.model.DataQuality), []string{BillingEstimateAmountOutOfRange})
		}
	}
	finalizeBillingModelSavings(&accumulator.model)
	if accumulator.discountSeen && !accumulator.model.MultipleDiscounts {
		value := accumulator.discountRatio
		accumulator.model.DiscountRatio = &value
	}
	if accumulator.hasContractDiscountFact && !accumulator.multipleContractDiscounts {
		value := accumulator.contractDiscountRatio
		accumulator.model.ContractDiscountRatio = &value
	}
	accumulator.model.MultipleContractDiscounts = accumulator.multipleContractDiscounts
}

func ensureBillingReconciliationQuality(target **BillingReconciliationDataQuality) *BillingReconciliationDataQuality {
	if *target == nil {
		*target = &BillingReconciliationDataQuality{Status: "complete"}
	}
	return *target
}

func accumulateBillingReconciliationQuality(target **BillingReconciliationDataQuality, source *BillingReconciliationDataQuality) {
	if source == nil {
		return
	}
	quality := ensureBillingReconciliationQuality(target)
	if source.EvidenceCoverage != nil {
		if quality.EvidenceCoverage == nil {
			quality.EvidenceCoverage = &BillingEvidenceCoverage{}
		}
		quality.EvidenceCoverage.Rows += source.EvidenceCoverage.Rows
		quality.EvidenceCoverage.GapRows += source.EvidenceCoverage.GapRows
		quality.EvidenceCoverage.AmountGapRows += source.EvidenceCoverage.AmountGapRows
		quality.EvidenceCoverage.UsageGapRows += source.EvidenceCoverage.UsageGapRows
		quality.EvidenceCoverage.OtherGapRows += source.EvidenceCoverage.OtherGapRows
	}
	quality.InputTokensUnavailableRequests += source.InputTokensUnavailableRequests
	quality.LegacyTestCacheReadRows += source.LegacyTestCacheReadRows
	quality.LegacyTestCacheWriteRows += source.LegacyTestCacheWriteRows
	quality.CacheReadUnavailableRequests += source.CacheReadUnavailableRequests
	quality.CacheReadUnreportedRequests += source.CacheReadUnreportedRequests
	quality.CacheWriteUnreportedRequests += source.CacheWriteUnreportedRequests
	quality.RecoveredBillingSecondsRows += source.RecoveredBillingSecondsRows
	quality.RefundedTaskHoldRows += source.RefundedTaskHoldRows
	quality.SecondsTaskLinkMissingRows += source.SecondsTaskLinkMissingRows
	quality.SecondsUnavailableRows += source.SecondsUnavailableRows
	quality.UnavailableRequests += source.UnavailableRequests
	quality.CacheWriteUnavailableRequests += source.CacheWriteUnavailableRequests
	quality.UnknownBillingModeRequests += source.UnknownBillingModeRequests
	quality.ProviderModelFallbackRows += source.ProviderModelFallbackRows
	quality.MissingHistoricalPriceRows += source.MissingHistoricalPriceRows
	quality.AuxiliaryChargeRows += source.AuxiliaryChargeRows
	quality.TestPricedRows += source.TestPricedRows
	quality.TestRecomputedRows += source.TestRecomputedRows
	quality.TestRecordedOriginalRows += source.TestRecordedOriginalRows
	quality.SecondsValueMissingRows += source.SecondsValueMissingRows
	quality.UsageWithoutAmountRows += source.UsageWithoutAmountRows
	if len(source.TestAmountPendingReasons) > 0 {
		if quality.TestAmountPendingReasons == nil {
			quality.TestAmountPendingReasons = make(map[string]int64)
		}
		for reason, count := range source.TestAmountPendingReasons {
			quality.TestAmountPendingReasons[reason] += count
		}
	}
}

// Missing test amounts are evidence gaps; known tests alone do not lower quality.
func finalizeBillingReconciliationQuality(target **BillingReconciliationDataQuality) {
	quality := ensureBillingReconciliationQuality(target)
	if quality.InputTokensUnavailableRequests > 0 || quality.CacheReadUnavailableRequests > 0 || quality.SecondsUnavailableRows > 0 || quality.UnavailableRequests > 0 || quality.CacheWriteUnavailableRequests > 0 || quality.UnknownBillingModeRequests > 0 || quality.ProviderModelFallbackRows > 0 || quality.MissingHistoricalPriceRows > 0 || quality.AuxiliaryChargeRows > 0 || quality.SecondsValueMissingRows > 0 || quality.UsageWithoutAmountRows > 0 {
		quality.Status = "partial"
	}
}

func accumulateBillingReconciliationUsage(target *BillingReconciliationUsage, source BillingReconciliationUsage) {
	target.Requests += source.Requests
	target.BillableCalls += source.BillableCalls
	target.RefundedCalls += source.RefundedCalls
	target.InputTokens += source.InputTokens
	target.CacheReadTokens += source.CacheReadTokens
	target.CacheWriteTokens += source.CacheWriteTokens
	target.OutputTokens += source.OutputTokens
	target.GrossQuota += source.GrossQuota
	target.RefundQuota += source.RefundQuota
}

func finalizeBillingReconciliationUsage(target *BillingReconciliationUsage) {
	// A refund-only period or model is a credit in the statement, not a zero
	// charge. Keeping the sign makes filtered rows add up to their parent total.
	target.NetQuota = target.GrossQuota - target.RefundQuota
}

type ProviderBillingPlatformSummary struct {
	ChannelId             int                               `json:"channel_id"`
	ChannelName           string                            `json:"channel_name"`
	ProviderModel         string                            `json:"provider_model"`
	CustomerModels        []string                          `json:"customer_models"`
	ProviderModelFallback bool                              `json:"provider_model_fallback,omitempty"`
	BillingMode           string                            `json:"billing_mode"`
	Usage                 ProviderBillingUsage              `json:"usage"`
	DataQuality           *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter          BillingReconciliationDetailFilter `json:"detail_filter"`
	// OriginalAmount is the signed official-price quota restored from the
	// customer settlement facts (consume positive, provable task-settlement
	// refunds negative). It stays nil unless every money-bearing row restored.
	OriginalAmount  *int64   `json:"original_amount,omitempty"`
	EstimateReasons []string `json:"estimate_reasons,omitempty"`
	// UsageOnly marks an item whose rows carry no customer settlement and no
	// confirmable test amount: usage stays, amounts are neither claimed nor
	// treated as a completeness gap for parents. Priced tests no longer make a
	// channel usage-only.
	UsageOnly          bool
	testMoneyRows      int
	originalQuota      decimal.Decimal
	referenceQuota     decimal.Decimal
	originalQuotaKnown bool
	originalComplete   bool
	moneyRows          int
	settlementRows     int
	detailModelName    string              `json:"-"`
	customerModels     map[string]struct{} `json:"-"`
}

// ProviderBillingUsage contains only usage facts persisted by this platform.
// It intentionally excludes customer quota and refund fields, which are not
// evidence of a supplier charge or credit.
type ProviderBillingUsage struct {
	Seconds                *decimal.Decimal `json:"seconds"`
	SecondsUnavailableRows int64            `json:"seconds_unavailable_rows,omitempty"`
	Requests               int64            `json:"requests"`
	BillableCalls          int64            `json:"billable_calls"`
	InputTokens            int64            `json:"input_tokens"`
	CacheReadTokens        int64            `json:"cache_read_tokens"`
	CacheWriteTokens       int64            `json:"cache_write_tokens"`
	OutputTokens           int64            `json:"output_tokens"`
}

type ProviderBillingDiscountProjection struct {
	Value        decimal.Decimal `json:"value"`
	Version      int64           `json:"version"`
	Source       string          `json:"source"`
	SourcePeriod int64           `json:"source_period,omitempty"`
}

type providerBillingSummaryKey struct {
	channelId int
	model     string
	mode      string
	fallback  bool
}

func accumulateProviderBillingLog(target *ProviderBillingUsage, log billingReconciliationLog, parsed parsedBillingReconciliationLog) (secondsMissing bool, secondsUnitKnown bool) {
	seconds, missing, unitKnown := upstreamBillingSeconds(log, parsed)
	if missing {
		target.SecondsUnavailableRows++
	}
	_ = seconds
	if seconds != nil {
		value := *seconds
		if target.Seconds != nil {
			value = target.Seconds.Add(value)
		}
		target.Seconds = &value
	}
	if parsed.isRequest {
		target.Requests += billingStatementRequestCount(parsed)
		if parsed.billingMode == BillingReconciliationModePerCall {
			target.BillableCalls++
		}
	}
	target.InputTokens += parsed.recordedInputTokens
	target.OutputTokens += parsed.outputTokens
	target.CacheReadTokens += parsed.cacheReadTokens
	target.CacheWriteTokens += parsed.cacheWrite.total
	return missing, unitKnown
}

func accumulateProviderBillingUsage(target *ProviderBillingUsage, source ProviderBillingUsage) {
	target.SecondsUnavailableRows += source.SecondsUnavailableRows
	if source.Seconds != nil {
		value := *source.Seconds
		if target.Seconds != nil {
			value = target.Seconds.Add(value)
		}
		target.Seconds = &value
	}
	target.Requests += source.Requests
	target.BillableCalls += source.BillableCalls
	target.InputTokens += source.InputTokens
	target.CacheReadTokens += source.CacheReadTokens
	target.CacheWriteTokens += source.CacheWriteTokens
	target.OutputTokens += source.OutputTokens
}

func previousBillingPeriodStart(periodStart int64) int64 {
	if periodStart <= 0 {
		return 0
	}
	settlementLocation := time.FixedZone("Asia/Shanghai", 8*60*60)
	period := time.Unix(periodStart, 0).In(settlementLocation)
	return time.Date(period.Year(), period.Month()-1, 1, 0, 0, 0, 0, settlementLocation).Unix()
}

func providerBillingEntityKey(periodStart int64, channelId int, providerModel string, billingMode string) string {
	return fmt.Sprintf("%d:%d:%s:%s", periodStart, channelId, providerModel, billingMode)
}

func createProviderBillingAudit(tx *gorm.DB, entityType string, entityKey string, action string, before any, after any, reason string, operatorId int) error {
	beforeJSON := []byte("null")
	var err error
	if before != nil {
		beforeJSON, err = common.Marshal(before)
		if err != nil {
			return err
		}
	}
	afterJSON, err := common.Marshal(after)
	if err != nil {
		return err
	}
	return tx.Create(&ProviderBillingAudit{
		EntityType: entityType,
		EntityHash: fmt.Sprintf("%x", common.Sha256Raw([]byte(entityKey))),
		EntityKey:  entityKey,
		Action:     action,
		Before:     string(beforeJSON),
		After:      string(afterJSON),
		Reason:     reason,
		OperatorId: operatorId,
	}).Error
}
