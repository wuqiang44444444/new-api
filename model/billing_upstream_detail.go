package model

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// UpstreamBillingDetailFilter selects the evidence rows behind the upstream
// reconciliation summary. It never filters or proves supplier identity: the
// URL grouping follows the channel's currently configured base URL.
type UpstreamBillingDetailFilter struct {
	EvidenceFilter        string
	Start, End            int64
	ChannelIds            []int
	ProviderModel         string
	ProviderModelFallback *bool
	BillingMode           string
	RequestId             string
	UpstreamRequestId     string
}

// UpstreamBillingDetailItem is one admin-only evidence row. The platform
// task id appears when present; the upstream task id is root-scoped in logs
// and is only included for root viewers. Missing IDs stay empty and display
// as "not recorded" instead of being guessed by time or amount.
type UpstreamBillingDetailItem struct {
	secondsRecovered      bool
	secondsLog            billingReconciliationLog
	SecondsSource         string           `json:"seconds_source,omitempty"`
	Seconds               *decimal.Decimal `json:"seconds"`
	RowId                 int64            `json:"row_id"`
	Time                  int64            `json:"time"`
	ChannelId             int              `json:"channel_id"`
	ChannelName           string           `json:"channel_name"`
	CustomerModel         string           `json:"customer_model"`
	ProviderModel         string           `json:"provider_model"`
	ProviderModelFallback bool             `json:"provider_model_fallback,omitempty"`
	BillingMode           string           `json:"billing_mode"`
	RecordedInputTokens   int64            `json:"recorded_input_tokens"`
	OutputTokens          int64            `json:"output_tokens"`
	CacheReadTokens       int64            `json:"cache_read_tokens"`
	CacheWriteTokens      int64            `json:"cache_write_tokens"`
	OriginalExactQuota    string           `json:"-"`
	OriginalAmount        *int64           `json:"original_amount,omitempty"`
	EstimateReasons       []string         `json:"estimate_reasons,omitempty"`
	RequestId             string           `json:"request_id"`
	UpstreamRequestId     string           `json:"upstream_request_id"`
	PlatformTaskId        string           `json:"platform_task_id,omitempty"`
	UpstreamTaskId        string           `json:"upstream_task_id,omitempty"`
	// TestPricing keeps the recorded test pricing evidence visible in details
	// and exports: the mode and settled/estimated status of the test fee. Nil
	// for non-test rows.
	TestPricing *UpstreamTestPricingProjection `json:"test_pricing,omitempty"`
	// Event classifies the log-row kind for display: call, task_create (hold),
	// task_adjustment (settlement), task_call (task row without a frozen
	// event), channel_test.
	Event       string                            `json:"event"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

// UpstreamBillingDetails is the paginated cross-customer detail view behind
// the upstream reconciliation page.
type UpstreamBillingDetails struct {
	Items    []UpstreamBillingDetailItem `json:"items"`
	Total    int64                       `json:"total"`
	Page     int                         `json:"page"`
	PageSize int                         `json:"page_size"`
}

// GetUpstreamBillingDetails scans one month of consume and refund rows across
// all customers, sharing the summary's signed amount scope. Provider model and
// billing mode filter after parsing; request IDs filter in SQL. Tests remain
// channel_test events.
func GetUpstreamBillingDetails(ctx context.Context, filter UpstreamBillingDetailFilter, page int, pageSize int, includeRootFields bool) (UpstreamBillingDetails, error) {
	result := UpstreamBillingDetails{Items: make([]UpstreamBillingDetailItem, 0), Page: page, PageSize: pageSize}
	skip := int64(page-1) * int64(pageSize)
	err := ScanUpstreamBillingDetails(ctx, filter, includeRootFields, BillingStatementReadPolicy{}, func(item UpstreamBillingDetailItem) error {
		if result.Total >= skip && len(result.Items) < pageSize {
			result.Items = append(result.Items, item)
		}
		result.Total++
		return nil
	})
	return result, err
}

// ScanUpstreamBillingDetails shares the parsed evidence and signed money scope
// with the summary. Exports supply a bounded read policy and never use UI pages.
func ScanUpstreamBillingDetails(ctx context.Context, filter UpstreamBillingDetailFilter, includeRootFields bool, policy BillingStatementReadPolicy, consume func(UpstreamBillingDetailItem) error) error {
	if err := ValidateUpstreamEvidenceFilter(filter.EvidenceFilter); err != nil {
		return err
	}
	names, err := getBillingURLChannelsById(filter.ChannelIds)
	if err != nil {
		return err
	}
	if len(filter.ChannelIds) == 0 {
		// Global category views still show current channel names. Select only the
		// display identity; no credentials or private channel configuration.
		var channels []Channel
		if err := DB.WithContext(ctx).Model(&Channel{}).Select("id, name").FindInBatches(&channels, 500, func(tx *gorm.DB, batch int) error {
			for _, channel := range channels {
				names[channel.Id] = channel
			}
			return nil
		}).Error; err != nil {
			return err
		}
	}
	var buffer []UpstreamBillingDetailItem
	flush := func() error {
		if err := flushRecoveredDetailSeconds(ctx, buffer); err != nil {
			return err
		}
		for _, item := range buffer {
			if matches, _ := matchUpstreamEvidenceFilter(item, filter.EvidenceFilter); !matches {
				continue
			}
			if err := consume(item); err != nil {
				return err
			}
		}
		buffer = buffer[:0]
		return nil
	}
	err = scanUpstreamBillingFacts(ctx, filter, policy, func(log Log, fact billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
		providerModel := strings.TrimSpace(parsed.providerModel)
		fallback := providerModel == ""
		if fallback {
			providerModel = log.ModelName
		}
		if (filter.ProviderModel != "" && providerModel != filter.ProviderModel) || (filter.ProviderModelFallback != nil && fallback != *filter.ProviderModelFallback) {
			return nil
		}
		if filter.BillingMode != "" && parsed.billingMode != filter.BillingMode {
			return nil
		}
		item := UpstreamBillingDetailItem{
			RowId: int64(log.Id), Time: log.CreatedAt, ChannelId: log.ChannelId, ChannelName: names[log.ChannelId].Name,
			CustomerModel: parsed.customerModel, ProviderModel: providerModel, ProviderModelFallback: fallback,
			BillingMode: parsed.billingMode, RecordedInputTokens: parsed.recordedInputTokens,
			OutputTokens: parsed.outputTokens, CacheReadTokens: parsed.cacheReadTokens, CacheWriteTokens: parsed.cacheWrite.total,
			RequestId: log.RequestId, UpstreamRequestId: log.UpstreamRequestId,
		}
		var secondsMissing, secondsUnitKnown bool
		item.Seconds, secondsMissing, secondsUnitKnown = upstreamBillingSeconds(fact, parsed)
		if secondsMissing {
			quality := ensureBillingReconciliationQuality(&item.DataQuality)
			quality.SecondsUnavailableRows = 1
			if secondsUnitKnown {
				quality.SecondsValueMissingRows = 1
			}
		}
		upstreamBillingDetailRowFacts(&item, fact, parsed, includeRootFields)
		buffer = append(buffer, item)
		if len(buffer) >= upstreamDetailFlushSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		return err
	}
	return flush()
}

// upstreamDetailFlushSize bounds one recovery batch of detail rows.
const upstreamDetailFlushSize = 500

// flushRecoveredDetailSeconds projects trusted task seconds into the buffered
// rows whose log evidence is missing but whose task identity can prove the
// value. Order is preserved: rows flush after recovery.
func flushRecoveredDetailSeconds(ctx context.Context, buffer []UpstreamBillingDetailItem) error {
	keys := []upstreamTaskSecondsKey{}
	for _, item := range buffer {
		if item.Seconds == nil && item.DataQuality != nil && item.DataQuality.SecondsValueMissingRows > 0 && item.PlatformTaskId != "" {
			keys = append(keys, upstreamTaskSecondsKey{item.secondsLog.UserId, item.secondsLog.ChannelId, item.PlatformTaskId})
		}
	}
	if len(keys) == 0 {
		return nil
	}
	tasks, err := resolveUpstreamTaskSeconds(ctx, keys)
	if err != nil {
		return err
	}
	for i := range buffer {
		item := &buffer[i]
		if item.Seconds != nil || item.DataQuality == nil || item.DataQuality.SecondsValueMissingRows == 0 {
			continue
		}
		task, ok := tasks[upstreamTaskSecondsKey{item.secondsLog.UserId, item.secondsLog.ChannelId, item.PlatformTaskId}]
		if !ok {
			continue
		}
		evidence := recoverUpstreamTaskSeconds(task, item.secondsLog)
		if evidence.source == "" {
			continue
		}
		item.Seconds, item.SecondsSource = evidence.seconds, evidence.source
		item.secondsRecovered = true
		item.DataQuality.SecondsValueMissingRows = 0
		item.DataQuality.SecondsUnavailableRows = 0
		if evidence.source == "billing_parameters" {
			item.DataQuality.RecoveredBillingSecondsRows = 1
		}
		if evidence.source == "refunded_hold" {
			item.Event = "task_refunded_hold"
			item.DataQuality.RefundedTaskHoldRows = 1
		}
		item.DataQuality.Status = "complete"
		finalizeBillingReconciliationQuality(&item.DataQuality)
	}
	return nil
}

// upstreamBillingDetailRowFacts fills the evidence fields of one detail row:
// the event kind, the per-row restored official-price amount and the task
// identities. Missing IDs stay empty; they are never guessed.
func upstreamBillingDetailRowFacts(item *UpstreamBillingDetailItem, fact billingReconciliationLog, parsed parsedBillingReconciliationLog, includeRootFields bool) {
	item.secondsLog = fact
	if item.Seconds != nil {
		item.SecondsSource = "recorded_usage"
	}
	var other map[string]json.RawMessage
	_ = common.UnmarshalJsonStr(fact.Other, &other)
	if other == nil {
		other = make(map[string]json.RawMessage)
	}
	taskId := billingBreakdownString(other["task_id"])
	event := billingBreakdownString(other["task_billing_event"])
	_, hasActual := other["actual_quota"]
	_, hasPre := other["pre_consumed_quota"]
	adjustment := hasActual || hasPre || event == "adjustment"

	switch {
	case isNativeChannelTestLog(fact.Type, fact.TokenId, fact.TokenName, fact.Content):
		item.Event = "channel_test"
	case fact.Type == LogTypeRefund && isProviderTaskUsageAdjustment(fact):
		item.Event = "task_adjustment"
	case fact.Type == LogTypeRefund:
		item.Event = "refund"
	case taskId != "" && event == "create" && !adjustment:
		item.Event = "task_create"
	case taskId != "" && (event == "adjustment" || adjustment):
		item.Event = "task_adjustment"
	case taskId != "":
		item.Event = "task_call"
	default:
		item.Event = "call"
	}

	if !parsed.isChannelTest {
		original, reasons := upstreamOriginalQuota(fact, parsed)
		item.EstimateReasons = reasons
		if len(reasons) == 0 {
			item.OriginalAmount = billingStatementOriginalQuota(original)
			if item.OriginalAmount == nil {
				item.EstimateReasons = []string{BillingEstimateAmountOutOfRange}
			} else {
				item.OriginalExactQuota = original.String()
			}
		}
	}

	quality := ensureBillingReconciliationQuality(&item.DataQuality)
	if quality.SecondsValueMissingRows > 0 && parsed.isTask && taskId == "" {
		quality.SecondsTaskLinkMissingRows = 1
	}
	if (fact.Type != LogTypeRefund || isProviderTaskUsageAdjustment(fact)) && !upstreamTaskCreatePreHold(parsed) {
		accumulateUpstreamCacheQuality(&item.DataQuality, parsed)
	}
	if item.ProviderModelFallback {
		quality.ProviderModelFallbackRows = 1
	}
	if parsed.billingMode == BillingReconciliationModeUnknown {
		quality.UnknownBillingModeRequests = 1
	}
	if parsed.unavailable {
		quality.UnavailableRequests = 1
	}
	if item.Event == "channel_test" {
		amount := upstreamTestAmountFor(fact, parsed)
		projection := upstreamTestPricingProjection(parsed, amount)
		item.TestPricing = &projection
		if amount.known {
			item.OriginalAmount = billingStatementOriginalQuota(amount.original)
			if item.OriginalAmount == nil {
				item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, BillingEstimateAmountOutOfRange)
			} else {
				item.OriginalExactQuota = amount.original.String()
				quality.TestPricedRows = 1
				if amount.replayed {
					quality.TestRecomputedRows = 1
				}
				if amount.recordedOriginal {
					quality.TestRecordedOriginalRows = 1
				}
			}
		} else {
			quality.UsageWithoutAmountRows = 1
			quality.TestAmountPendingReasons = map[string]int64{amount.reason: 1}
			if parsed.billingMode == BillingReconciliationModeToken {
				if parsed.cacheReadEvidence == upstreamCacheLegacyUnknown {
					quality.LegacyTestCacheReadRows++
				}
				if parsed.cacheWriteEvidence == upstreamCacheLegacyUnknown {
					quality.LegacyTestCacheWriteRows++
				}
			}
			item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, BillingEstimateTestAmountPending)
		}
	}
	if len(item.EstimateReasons) > 0 {
		accumulateBillingEstimateReasonQuality(quality, item.EstimateReasons)
	}
	finalizeBillingReconciliationQuality(&item.DataQuality)
	item.PlatformTaskId = taskId
	if includeRootFields {
		item.UpstreamTaskId = billingBreakdownString(other["upstream_task_id"])
	}
}
