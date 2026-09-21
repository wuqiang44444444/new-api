package model

import (
	"context"
	"github.com/shopspring/decimal"
)

// The single inheritance rule used by both explicit initialization and the
// read-only export snapshot. Version zero means no current-month row exists.
func inheritedProviderChannelDiscount(period int64, id int, source ProviderChannelBillingDiscount, found bool) ProviderChannelBillingDiscount {
	result := ProviderChannelBillingDiscount{PeriodStart: period, ChannelId: id, Discount: decimal.NewFromInt(1), Reason: reasonChannelDiscountDefault}
	if found && source.PendingReason == "" {
		result.Discount = source.Discount
		result.CopiedFromPeriod = source.PeriodStart
		result.Reason = reasonChannelDiscountAutoCopy
	}
	return result
}

func upstreamExportDiscounts(ctx context.Context, period int64, ids []int) (map[int]ProviderChannelBillingDiscount, error) {
	result := map[int]ProviderChannelBillingDiscount{}
	for offset := 0; offset < len(ids); offset += 100 {
		batch := ids[offset:min(offset+100, len(ids))]
		var rows []ProviderChannelBillingDiscount
		if err := DB.WithContext(ctx).Where("period_start IN ? AND channel_id IN ?", []int64{period, previousBillingPeriodStart(period)}, batch).Find(&rows).Error; err != nil {
			return nil, err
		}
		current, previous := map[int]ProviderChannelBillingDiscount{}, map[int]ProviderChannelBillingDiscount{}
		for _, row := range rows {
			if row.PeriodStart == period {
				current[row.ChannelId] = row
			} else {
				previous[row.ChannelId] = row
			}
		}
		for _, id := range batch {
			if record, exists := current[id]; exists {
				if record.PendingReason == "" {
					result[id] = record
				}
				continue
			}
			source, found := previous[id]
			result[id] = inheritedProviderChannelDiscount(period, id, source, found)
		}
	}
	return result, nil
}
