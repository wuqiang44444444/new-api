package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	BillingReconciliationModeToken   = "token"
	BillingReconciliationModePerCall = "per_call"
	BillingReconciliationModeUnknown = "unknown"
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
	return DB.AutoMigrate(
		&ProviderBillingDiscount{},
		&ProviderBillingAudit{},
	)
}

type BillingReconciliationDataQuality struct {
	Status                     string `json:"status"`
	UnavailableRequests        int64  `json:"unavailable_requests,omitempty"`
	UnknownBillingModeRequests int64  `json:"unknown_billing_mode_requests,omitempty"`
	ProviderModelFallbackRows  int64  `json:"provider_model_fallback_rows,omitempty"`
	MissingHistoricalPriceRows int64  `json:"missing_historical_price_rows,omitempty"`
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
	ModelName         string                            `json:"model_name"`
	BillingMode       string                            `json:"billing_mode"`
	Usage             BillingReconciliationUsage        `json:"usage"`
	OriginalQuota     *int64                            `json:"original_quota,omitempty"`
	DiscountRatio     *float64                          `json:"discount_ratio,omitempty"`
	MultipleDiscounts bool                              `json:"multiple_discounts,omitempty"`
	PriceVersions     int64                             `json:"price_versions"`
	DataQuality       *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter      BillingReconciliationDetailFilter `json:"detail_filter"`
}

type BillingReconciliationGroupSummary struct {
	Id            int64                               `json:"id"`
	Name          string                              `json:"name"`
	Usage         BillingReconciliationUsage          `json:"usage"`
	OriginalQuota *int64                              `json:"original_quota,omitempty"`
	DiscountQuota *int64                              `json:"discount_quota,omitempty"`
	Models        []BillingReconciliationModelSummary `json:"models"`
	Deleted       bool                                `json:"deleted,omitempty"`
}

type BillingReconciliationDetailFilter struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	UserId         int    `json:"user_id,omitempty"`
	TokenId        int    `json:"token_id,omitempty"`
	ChannelId      int    `json:"channel_id,omitempty"`
	ModelName      string `json:"model_name,omitempty"`
	BillingMode    string `json:"billing_mode,omitempty"`
}

type BillingCustomerStatement struct {
	UserId         int                                 `json:"user_id"`
	Username       string                              `json:"username"`
	DisplayName    string                              `json:"display_name"`
	Deleted        bool                                `json:"deleted,omitempty"`
	Dimension      string                              `json:"dimension"`
	CurrentBalance int                                 `json:"current_balance"`
	Summary        BillingReconciliationUsage          `json:"summary"`
	OriginalQuota  *int64                              `json:"original_quota,omitempty"`
	DiscountQuota  *int64                              `json:"discount_quota,omitempty"`
	Groups         []BillingReconciliationGroupSummary `json:"groups"`
	DataQuality    *BillingReconciliationDataQuality   `json:"data_quality,omitempty"`
}

type billingReconciliationLog struct {
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
	model                 BillingReconciliationModelSummary
	discountSeen          bool
	discountRatio         float64
	originalQuota         int64
	originalQuotaKnown    bool
	originalQuotaComplete bool
	priceSnapshotMarkers  map[string]struct{}
}

func GetBillingCustomerStatement(
	userId int,
	startTimestamp int64,
	endTimestamp int64,
	dimension string,
	groupId int,
	modelName string,
	billingMode string,
) (BillingCustomerStatement, error) {
	statement := BillingCustomerStatement{
		UserId:    userId,
		Dimension: dimension,
		Groups:    make([]BillingReconciliationGroupSummary, 0),
	}
	if dimension != "api_key" && dimension != "channel" {
		return statement, errors.New("invalid billing dimension")
	}
	if billingMode != "" && billingMode != BillingReconciliationModeToken && billingMode != BillingReconciliationModePerCall && billingMode != BillingReconciliationModeUnknown {
		return statement, errors.New("invalid billing mode")
	}

	query := LOG_DB.Model(&Log{}).
		Select("user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(other, '') AS other").
		Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at <= ?", userId, []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp)
	if modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	if groupId > 0 {
		if dimension == "api_key" {
			query = query.Where("token_id = ?", groupId)
		} else {
			query = query.Where("channel_id = ?", groupId)
		}
	}

	rows, err := query.Rows()
	if err != nil {
		return statement, err
	}
	defer rows.Close()

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

	for rows.Next() {
		var log billingReconciliationLog
		if err := rows.Scan(&log.UserId, &log.TokenId, &log.TokenName, &log.ChannelId, &log.ModelName, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Other); err != nil {
			return statement, err
		}
		parsed := parseBillingReconciliationLog(log)
		if billingMode != "" && parsed.billingMode != billingMode {
			continue
		}

		selectedGroupId := int64(log.TokenId)
		selectedGroupName := log.TokenName
		if dimension == "channel" {
			selectedGroupId = int64(log.ChannelId)
			selectedGroupName = ""
		}
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
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&accumulator.model.DataQuality).UnavailableRequests++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&accumulator.model.DataQuality).UnknownBillingModeRequests++
		}
		accumulateBillingReconciliationPrice(accumulator, log, parsed)
	}
	if err := rows.Err(); err != nil {
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
		accumulateBillingReconciliationQuality(&statement.DataQuality, accumulator.model.DataQuality)
		group := groups[groupKey{id: key.groupId}]
		group.Models = append(group.Models, accumulator.model)
		accumulateBillingReconciliationUsage(&group.Usage, accumulator.model.Usage)
	}
	for key, group := range groups {
		name, ok := groupNames[key.id]
		if !ok || strings.TrimSpace(name) == "" {
			group.Deleted = key.id > 0
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
	originalQuota := int64(0)
	for _, item := range group.Models {
		if item.Usage.GrossQuota > 0 && item.OriginalQuota == nil {
			return
		}
		if item.OriginalQuota != nil {
			originalQuota += *item.OriginalQuota
		}
	}
	discountQuota := originalQuota - group.Usage.GrossQuota
	group.OriginalQuota = &originalQuota
	group.DiscountQuota = &discountQuota
}

func finalizeBillingCustomerStatementOriginalQuota(statement *BillingCustomerStatement) {
	if statement == nil {
		return
	}
	originalQuota := int64(0)
	for _, group := range statement.Groups {
		if group.Usage.GrossQuota > 0 && group.OriginalQuota == nil {
			return
		}
		if group.OriginalQuota != nil {
			originalQuota += *group.OriginalQuota
		}
	}
	discountQuota := originalQuota - statement.Summary.GrossQuota
	statement.OriginalQuota = &originalQuota
	statement.DiscountQuota = &discountQuota
}

type parsedBillingReconciliationLog struct {
	billingMode     string
	cacheReadTokens int64
	cacheWrite      billingStatementCacheWriteTokens
	discountRatio   *float64
	priceMarker     string
	providerModel   string
	unavailable     bool
}

func parseBillingReconciliationLog(log billingReconciliationLog) parsedBillingReconciliationLog {
	parsed := parsedBillingReconciliationLog{billingMode: BillingReconciliationModeUnknown}
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
	parsed.cacheReadTokens, _ = billingBreakdownNonNegativeInt(other["cache_tokens"])
	parsed.cacheWrite = normalizedBillingBreakdownCacheWriteTokens(other)
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
	var snapshot map[string]json.RawMessage
	if len(snapshotRaw) > 0 {
		if common.Unmarshal(snapshotRaw, &snapshot) == nil {
			snapshotMode := billingBreakdownString(snapshot["billing_mode"])
			if snapshotMode == BillingReconciliationModeToken || snapshotMode == BillingReconciliationModePerCall {
				parsed.billingMode = snapshotMode
			}
			if providerModel := billingBreakdownString(snapshot["provider_model"]); providerModel != "" {
				parsed.providerModel = providerModel
			}
		}
	}
	requestPath := billingBreakdownString(other["request_path"])
	isTask, _ := billingReconciliationBool(other["is_task"])
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
	modelRatio, modelRatioOk := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "model_ratio"))
	if parsed.billingMode == BillingReconciliationModeUnknown && modelRatioOk && modelRatio > 0 {
		parsed.billingMode = BillingReconciliationModeToken
	}
	if parsed.billingMode == BillingReconciliationModeUnknown && (log.PromptTokens > 0 || log.CompletionTokens > 0 || parsed.cacheReadTokens > 0 || parsed.cacheWrite.total > 0) {
		parsed.billingMode = BillingReconciliationModeToken
	}
	if ratio, ok := billingReconciliationFloat(billingReconciliationSnapshotRaw(snapshot, other, "group_ratio")); ok && ratio > 0 {
		parsed.discountRatio = &ratio
	}
	priceMarkers := []string{
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "model_price")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "model_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "completion_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "cache_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "group_ratio")),
		billingReconciliationRawMarker(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64")),
	}
	for _, marker := range priceMarkers {
		if marker != "" {
			parsed.priceMarker = strings.Join(priceMarkers, "|")
			break
		}
	}
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
	if err := common.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	return value, true
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
		if !strings.Contains(log.Other, `"task_id"`) {
			target.Requests++
			if parsed.billingMode == BillingReconciliationModePerCall {
				target.BillableCalls++
			}
		}
		target.GrossQuota += quota
	} else if log.Type == LogTypeRefund {
		if !strings.Contains(log.Other, `"task_id"`) && parsed.billingMode == BillingReconciliationModePerCall {
			target.RefundedCalls++
		}
		target.RefundQuota += quota
	}
	target.InputTokens += max(int64(log.PromptTokens), int64(0))
	target.OutputTokens += max(int64(log.CompletionTokens), int64(0))
	target.CacheReadTokens += parsed.cacheReadTokens
	target.CacheWriteTokens += parsed.cacheWrite.total
}

func accumulateBillingReconciliationPrice(accumulator *billingReconciliationModelAccumulator, log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	if log.Type != LogTypeConsume {
		return
	}
	if parsed.priceMarker != "" {
		accumulator.priceSnapshotMarkers[parsed.priceMarker] = struct{}{}
	}
	quota := max(int64(log.Quota), int64(0))
	if quota == 0 {
		return
	}
	if parsed.discountRatio == nil || *parsed.discountRatio <= 0 {
		accumulator.originalQuotaComplete = false
		ensureBillingReconciliationQuality(&accumulator.model.DataQuality).MissingHistoricalPriceRows++
		return
	}
	ratio := *parsed.discountRatio
	if !accumulator.discountSeen {
		accumulator.discountSeen = true
		accumulator.discountRatio = ratio
	} else if accumulator.discountRatio != ratio {
		accumulator.model.MultipleDiscounts = true
	}
	original, _ := common.QuotaRoundChecked(float64(quota) / ratio)
	accumulator.originalQuota += int64(max(original, 0))
	accumulator.originalQuotaKnown = true
}

func finalizeBillingReconciliationPrice(accumulator *billingReconciliationModelAccumulator) {
	accumulator.model.PriceVersions = int64(len(accumulator.priceSnapshotMarkers))
	if accumulator.originalQuotaKnown && accumulator.originalQuotaComplete {
		value := accumulator.originalQuota
		accumulator.model.OriginalQuota = &value
	}
	if accumulator.discountSeen && !accumulator.model.MultipleDiscounts {
		value := accumulator.discountRatio
		accumulator.model.DiscountRatio = &value
	}
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
	quality.UnavailableRequests += source.UnavailableRequests
	quality.UnknownBillingModeRequests += source.UnknownBillingModeRequests
	quality.ProviderModelFallbackRows += source.ProviderModelFallbackRows
	quality.MissingHistoricalPriceRows += source.MissingHistoricalPriceRows
}

func finalizeBillingReconciliationQuality(target **BillingReconciliationDataQuality) {
	quality := ensureBillingReconciliationQuality(target)
	if quality.UnavailableRequests > 0 || quality.UnknownBillingModeRequests > 0 || quality.ProviderModelFallbackRows > 0 || quality.MissingHistoricalPriceRows > 0 {
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
	if target.GrossQuota > target.RefundQuota {
		target.NetQuota = target.GrossQuota - target.RefundQuota
	} else {
		target.NetQuota = 0
	}
}

type ProviderBillingPlatformSummary struct {
	ChannelId             int                               `json:"channel_id"`
	ChannelName           string                            `json:"channel_name"`
	ProviderModel         string                            `json:"provider_model"`
	ProviderModelFallback bool                              `json:"provider_model_fallback,omitempty"`
	BillingMode           string                            `json:"billing_mode"`
	Usage                 ProviderBillingUsage              `json:"usage"`
	Discount              ProviderBillingDiscountProjection `json:"discount"`
	DataQuality           *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter          BillingReconciliationDetailFilter `json:"detail_filter"`
	detailModelName       string                            `json:"-"`
}

// ProviderBillingUsage contains only usage facts persisted by this platform.
// It intentionally excludes customer quota and refund fields, which are not
// evidence of a supplier charge or credit.
type ProviderBillingUsage struct {
	Requests         int64 `json:"requests"`
	BillableCalls    int64 `json:"billable_calls"`
	InputTokens      int64 `json:"input_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
}

type ProviderBillingDiscountProjection struct {
	Value        decimal.Decimal `json:"value"`
	Version      int64           `json:"version"`
	Source       string          `json:"source"`
	SourcePeriod int64           `json:"source_period,omitempty"`
}

type ProviderBillingChannelSummary struct {
	ChannelId   int                               `json:"channel_id"`
	ChannelName string                            `json:"channel_name"`
	Usage       ProviderBillingUsage              `json:"usage"`
	Models      []ProviderBillingPlatformSummary  `json:"models"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

type ProviderBillingSummary struct {
	Channels    []ProviderBillingChannelSummary   `json:"channels"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

type providerBillingSummaryKey struct {
	channelId int
	model     string
	mode      string
	fallback  bool
}

func GetProviderBillingSummary(startTimestamp int64, endTimestamp int64, periodStart int64, channelId int, modelName string, billingMode string, operatorId int) (ProviderBillingSummary, error) {
	summary := ProviderBillingSummary{
		Channels: make([]ProviderBillingChannelSummary, 0),
	}
	query := LOG_DB.Model(&Log{}).
		Select("user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(other, '') AS other").
		Where("type = ? AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTimestamp, endTimestamp)
	if channelId > 0 {
		query = query.Where("channel_id = ?", channelId)
	}
	rows, err := query.Rows()
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	platform := make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary)
	for rows.Next() {
		var log billingReconciliationLog
		if err := rows.Scan(&log.UserId, &log.TokenId, &log.TokenName, &log.ChannelId, &log.ModelName, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Other); err != nil {
			return summary, err
		}
		parsed := parseBillingReconciliationLog(log)
		providerModel := strings.TrimSpace(parsed.providerModel)
		fallback := false
		if providerModel == "" {
			providerModel = log.ModelName
			fallback = true
		}
		if modelName != "" && providerModel != modelName {
			continue
		}
		if billingMode != "" && parsed.billingMode != billingMode {
			continue
		}
		itemKey := providerBillingSummaryKey{channelId: log.ChannelId, model: providerModel, mode: parsed.billingMode, fallback: fallback}
		item, ok := platform[itemKey]
		if !ok {
			item = &ProviderBillingPlatformSummary{
				ChannelId:             log.ChannelId,
				ProviderModel:         providerModel,
				ProviderModelFallback: fallback,
				BillingMode:           parsed.billingMode,
				DetailFilter: BillingReconciliationDetailFilter{
					StartTimestamp: startTimestamp,
					EndTimestamp:   endTimestamp,
					ChannelId:      log.ChannelId,
					ModelName:      log.ModelName,
					BillingMode:    parsed.billingMode,
				},
				detailModelName: log.ModelName,
			}
			platform[itemKey] = item
		} else if item.detailModelName != log.ModelName {
			item.DetailFilter.ModelName = ""
		}
		accumulateProviderBillingLog(&item.Usage, log, parsed)
		if fallback {
			ensureBillingReconciliationQuality(&item.DataQuality).ProviderModelFallbackRows++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&item.DataQuality).UnknownBillingModeRequests++
		}
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&item.DataQuality).UnavailableRequests++
		}
	}
	if err := rows.Err(); err != nil {
		return summary, err
	}

	discounts, err := getProviderBillingDiscountProjections(periodStart, platform, operatorId)
	if err != nil {
		return summary, err
	}

	channelIds := make([]int, 0)
	channelIdSet := make(map[int]struct{})
	for itemKey := range platform {
		if _, ok := channelIdSet[itemKey.channelId]; !ok && itemKey.channelId > 0 {
			channelIds = append(channelIds, itemKey.channelId)
			channelIdSet[itemKey.channelId] = struct{}{}
		}
	}
	channelNames := make(map[int]string)
	if len(channelIds) > 0 {
		var channels []struct {
			Id   int
			Name string
		}
		if err := DB.Model(&Channel{}).Select("id, name").Where("id IN ?", channelIds).Scan(&channels).Error; err != nil {
			return summary, err
		}
		for _, channel := range channels {
			channelNames[channel.Id] = channel.Name
		}
	}

	channels := make(map[int]*ProviderBillingChannelSummary)
	for itemKey, item := range platform {
		finalizeBillingReconciliationQuality(&item.DataQuality)
		accumulateBillingReconciliationQuality(&summary.DataQuality, item.DataQuality)
		item.Discount = discounts[itemKey]
		name := channelNames[itemKey.channelId]
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("Channel #%d", itemKey.channelId)
		}
		item.ChannelName = name
		channel, ok := channels[itemKey.channelId]
		if !ok {
			channel = &ProviderBillingChannelSummary{
				ChannelId:   itemKey.channelId,
				ChannelName: name,
				Models:      make([]ProviderBillingPlatformSummary, 0),
			}
			channels[itemKey.channelId] = channel
		}
		accumulateProviderBillingUsage(&channel.Usage, item.Usage)
		accumulateBillingReconciliationQuality(&channel.DataQuality, item.DataQuality)
		channel.Models = append(channel.Models, *item)
	}
	for _, channel := range channels {
		finalizeBillingReconciliationQuality(&channel.DataQuality)
		sort.Slice(channel.Models, func(i, j int) bool {
			if channel.Models[i].ProviderModel != channel.Models[j].ProviderModel {
				return channel.Models[i].ProviderModel < channel.Models[j].ProviderModel
			}
			return channel.Models[i].BillingMode < channel.Models[j].BillingMode
		})
		summary.Channels = append(summary.Channels, *channel)
	}
	sort.Slice(summary.Channels, func(i, j int) bool { return summary.Channels[i].ChannelName < summary.Channels[j].ChannelName })
	finalizeBillingReconciliationQuality(&summary.DataQuality)
	return summary, nil
}

func accumulateProviderBillingLog(target *ProviderBillingUsage, log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	if !strings.Contains(log.Other, `"task_id"`) {
		target.Requests++
		if parsed.billingMode == BillingReconciliationModePerCall {
			target.BillableCalls++
		}
	}
	target.InputTokens += max(int64(log.PromptTokens), int64(0))
	target.OutputTokens += max(int64(log.CompletionTokens), int64(0))
	target.CacheReadTokens += parsed.cacheReadTokens
	target.CacheWriteTokens += parsed.cacheWrite.total
}

func accumulateProviderBillingUsage(target *ProviderBillingUsage, source ProviderBillingUsage) {
	target.Requests += source.Requests
	target.BillableCalls += source.BillableCalls
	target.InputTokens += source.InputTokens
	target.CacheReadTokens += source.CacheReadTokens
	target.CacheWriteTokens += source.CacheWriteTokens
	target.OutputTokens += source.OutputTokens
}

func getProviderBillingDiscountProjections(periodStart int64, items map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, operatorId int) (map[providerBillingSummaryKey]ProviderBillingDiscountProjection, error) {
	if err := materializeProviderBillingDiscounts(periodStart, items, operatorId); err != nil {
		return nil, err
	}
	result := make(map[providerBillingSummaryKey]ProviderBillingDiscountProjection, len(items))
	var current []ProviderBillingDiscount
	if err := DB.Where("period_start = ?", periodStart).Find(&current).Error; err != nil {
		return nil, err
	}
	for _, discount := range current {
		key := providerBillingSummaryKey{channelId: discount.ChannelId, model: discount.ProviderModel, mode: discount.BillingMode}
		source := "database"
		sourcePeriod := periodStart
		if discount.CopiedFromPeriod > 0 && discount.Version == 1 {
			source = "previous_period"
			sourcePeriod = discount.CopiedFromPeriod
		}
		result[key] = ProviderBillingDiscountProjection{Value: discount.Discount, Version: discount.Version, Source: source, SourcePeriod: sourcePeriod}
	}
	for key := range items {
		if _, ok := result[key]; !ok {
			result[key] = ProviderBillingDiscountProjection{Value: decimal.NewFromInt(1), Source: "default"}
		}
	}
	return result, nil
}

func materializeProviderBillingDiscounts(periodStart int64, items map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, operatorId int) error {
	if len(items) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		channelIds := make([]int, 0, len(items))
		seenChannels := make(map[int]struct{})
		for key := range items {
			if key.channelId > 0 {
				if _, exists := seenChannels[key.channelId]; !exists {
					channelIds = append(channelIds, key.channelId)
					seenChannels[key.channelId] = struct{}{}
				}
			}
		}
		if len(channelIds) > 0 {
			sort.Ints(channelIds)
			var channels []Channel
			if err := lockForUpdate(tx).Select("id").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
				return err
			}
		}

		var current []ProviderBillingDiscount
		if err := tx.Where("period_start = ?", periodStart).Find(&current).Error; err != nil {
			return err
		}
		currentKeys := make(map[providerBillingSummaryKey]struct{}, len(current))
		for _, discount := range current {
			currentKeys[providerBillingSummaryKey{channelId: discount.ChannelId, model: discount.ProviderModel, mode: discount.BillingMode}] = struct{}{}
		}

		previousPeriod := previousBillingPeriodStart(periodStart)
		var previous []ProviderBillingDiscount
		if previousPeriod > 0 {
			if err := tx.Where("period_start = ?", previousPeriod).Find(&previous).Error; err != nil {
				return err
			}
		}
		previousByKey := make(map[providerBillingSummaryKey]ProviderBillingDiscount, len(previous))
		for _, discount := range previous {
			previousByKey[providerBillingSummaryKey{channelId: discount.ChannelId, model: discount.ProviderModel, mode: discount.BillingMode}] = discount
		}

		for key := range items {
			if key.channelId <= 0 || (key.mode != BillingReconciliationModeToken && key.mode != BillingReconciliationModePerCall) {
				continue
			}
			if item := items[key]; item != nil && item.DataQuality != nil && item.DataQuality.ProviderModelFallbackRows > 0 {
				continue
			}
			if _, exists := currentKeys[key]; exists {
				continue
			}
			discount := ProviderBillingDiscount{
				PeriodStart: periodStart, ChannelId: key.channelId, ProviderModel: key.model, BillingMode: key.mode,
				Discount: decimal.NewFromInt(1), Version: 1,
				Reason: "automatic monthly default", CreatedBy: operatorId, UpdatedBy: operatorId,
			}
			if previousDiscount, exists := previousByKey[key]; exists {
				discount.Discount = previousDiscount.Discount
				discount.CopiedFromPeriod = previousPeriod
				discount.Reason = "automatic copy from previous billing period"
			}
			if err := tx.Create(&discount).Error; err != nil {
				return err
			}
			if err := createProviderBillingAudit(tx, "discount", providerBillingEntityKey(discount.PeriodStart, discount.ChannelId, discount.ProviderModel, discount.BillingMode), "create", nil, &discount, discount.Reason, operatorId); err != nil {
				return err
			}
		}
		return nil
	})
}

func previousBillingPeriodStart(periodStart int64) int64 {
	if periodStart <= 0 {
		return 0
	}
	settlementLocation := time.FixedZone("Asia/Shanghai", 8*60*60)
	period := time.Unix(periodStart, 0).In(settlementLocation)
	return time.Date(period.Year(), period.Month()-1, 1, 0, 0, 0, 0, settlementLocation).Unix()
}

func SaveProviderBillingDiscount(discount *ProviderBillingDiscount, expectedVersion int64, operatorId int) error {
	if discount == nil {
		return errors.New("provider billing discount is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing ProviderBillingDiscount
		err := lockForUpdate(tx).Where("period_start = ? AND channel_id = ? AND provider_model = ? AND billing_mode = ?", discount.PeriodStart, discount.ChannelId, discount.ProviderModel, discount.BillingMode).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expectedVersion != 0 {
				return ErrBillingReconciliationVersionConflict
			}
			discount.Version = 1
			discount.CreatedBy = operatorId
			discount.UpdatedBy = operatorId
			if err := tx.Create(discount).Error; err != nil {
				return err
			}
			return createProviderBillingAudit(tx, "discount", providerBillingEntityKey(discount.PeriodStart, discount.ChannelId, discount.ProviderModel, discount.BillingMode), "create", nil, discount, discount.Reason, operatorId)
		}
		if err != nil {
			return err
		}
		if existing.Version != expectedVersion {
			return ErrBillingReconciliationVersionConflict
		}
		result := tx.Model(&ProviderBillingDiscount{}).Where("id = ? AND version = ?", existing.Id, expectedVersion).Updates(map[string]interface{}{
			"discount": discount.Discount, "copied_from_period": discount.CopiedFromPeriod, "reason": discount.Reason,
			"version": existing.Version + 1, "updated_by": operatorId,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBillingReconciliationVersionConflict
		}
		discount.Id = existing.Id
		discount.Version = existing.Version + 1
		discount.CreatedBy = existing.CreatedBy
		discount.UpdatedBy = operatorId
		return createProviderBillingAudit(tx, "discount", providerBillingEntityKey(discount.PeriodStart, discount.ChannelId, discount.ProviderModel, discount.BillingMode), "update", &existing, discount, discount.Reason, operatorId)
	})
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
