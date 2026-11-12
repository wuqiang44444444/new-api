package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

type upstreamBillingAccumulator struct {
	items                        map[providerBillingSummaryKey]*ProviderBillingPlatformSummary
	taskCache                    *upstreamTaskCacheTracker
	coverage                     upstreamEvidenceCoverage
	discounts                    map[int]ProviderChannelBillingDiscount
	startTimestamp, endTimestamp int64
	maxGroups                    int
	observeBasis                 func(providerBillingSummaryKey, decimal.Decimal)
}

func newUpstreamBillingAccumulator(start, end int64, discounts map[int]ProviderChannelBillingDiscount, maxGroups int) *upstreamBillingAccumulator {
	return &upstreamBillingAccumulator{
		items:     make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary),
		taskCache: newUpstreamTaskCacheTracker(), discounts: discounts,
		startTimestamp: start, endTimestamp: end, maxGroups: maxGroups,
	}
}

func (a *upstreamBillingAccumulator) observe(ctx context.Context, row Log, log billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
	startTimestamp, endTimestamp := a.startTimestamp, a.endTimestamp
	providerModel := strings.TrimSpace(parsed.providerModel)
	fallback := false
	if providerModel == "" {
		providerModel = log.ModelName
		fallback = true
	}
	itemKey := providerBillingSummaryKey{channelId: log.ChannelId, model: providerModel, mode: parsed.billingMode, fallback: fallback}
	item, ok := a.items[itemKey]
	if !ok {
		if a.maxGroups > 0 && len(a.items) >= a.maxGroups {
			return fmt.Errorf("upstream summary group limit exceeded")
		}
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
			detailModelName:  log.ModelName,
			customerModels:   make(map[string]struct{}),
			originalComplete: true,
		}
		a.items[itemKey] = item
	} else if item.detailModelName != log.ModelName {
		item.DetailFilter.ModelName = ""
	}
	if log.ModelName != "" {
		item.customerModels[log.ModelName] = struct{}{}
	}
	if log.Type != LogTypeRefund || isProviderTaskUsageAdjustment(log) {
		secondsMissing, secondsUnitKnown := accumulateProviderBillingLog(&item.Usage, log, parsed)
		if secondsMissing {
			quality := ensureBillingReconciliationQuality(&item.DataQuality)
			quality.SecondsUnavailableRows++
			if secondsUnitKnown {
				quality.SecondsValueMissingRows++
			}
		}
		// Cache quality measures final metering rows: creation pre-holds are
		// estimates and one task's rows dedupe to a single missing count.
		if !upstreamTaskCreatePreHold(parsed) {
			if parsed.taskID != "" {
				a.taskCache.observe(itemKey, log.UserId, parsed)
			} else {
				accumulateUpstreamCacheQuality(&item.DataQuality, parsed)
			}
		}

	}
	discount, _ := providerChannelBillingDiscountFor(a.discounts, 0, log.ChannelId)
	originalBefore := item.originalQuota
	accumulateProviderBillingItemAmount(item, log, parsed, discount.Changes.valueAt(discount.Discount, log.CreatedAt))
	if a.observeBasis != nil {
		delta := item.originalQuota.Sub(originalBefore)
		if !delta.IsZero() {
			a.observeBasis(itemKey, delta)
		}
	}
	if fallback {
		ensureBillingReconciliationQuality(&item.DataQuality).ProviderModelFallbackRows++
	}
	if parsed.billingMode == BillingReconciliationModeUnknown {
		ensureBillingReconciliationQuality(&item.DataQuality).UnknownBillingModeRequests++
	}
	if parsed.unavailable {
		ensureBillingReconciliationQuality(&item.DataQuality).UnavailableRequests++
	}
	a.coverage.observe(itemKey, row, log, parsed, a.coverage.onRow != nil)
	if len(a.coverage.rows) >= upstreamDetailFlushSize {
		return a.coverage.flush(ctx, a.items)
	}
	return nil
}
