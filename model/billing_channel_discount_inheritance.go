package model

import (
	"context"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The single inheritance rule used by both explicit initialization and the
// read-only report projections. Version zero means no current-month row exists.
func inheritedProviderChannelDiscount(period int64, id int, source ProviderChannelBillingDiscount, found bool) ProviderChannelBillingDiscount {
	result := ProviderChannelBillingDiscount{PeriodStart: period, ChannelId: id, Discount: decimal.NewFromInt(1), Reason: reasonChannelDiscountDefault}
	if found && source.PendingReason == "" {
		result.Discount = source.Discount
		result.CopiedFromPeriod = source.PeriodStart
		result.Reason = reasonChannelDiscountAutoCopy
	}
	return result
}

// loadProviderChannelBillingDiscounts resolves the same read-only month facts
// for pages, day/week analytics and exports, including deleted channels.
// Current conflicts stay pending; only an absent current row inherits.
func loadProviderChannelBillingDiscounts(ctx context.Context, period int64, ids []int) (map[int]ProviderChannelBillingDiscount, error) {
	current, previous := map[int]ProviderChannelBillingDiscount{}, map[int]ProviderChannelBillingDiscount{}
	for offset := 0; offset < len(ids) || offset == 0; offset += 100 {
		query := DB.WithContext(ctx).Where("period_start IN ?", []int64{period, previousBillingPeriodStart(period)})
		if len(ids) > 0 {
			query = query.Where("channel_id IN ?", ids[offset:min(offset+100, len(ids))])
		}
		var rows []ProviderChannelBillingDiscount
		if err := query.FindInBatches(&rows, 500, func(_ *gorm.DB, _ int) error {
			for _, row := range rows {
				if row.PeriodStart == period {
					current[row.ChannelId] = row
				} else {
					previous[row.ChannelId] = row
				}
			}
			return nil
		}).Error; err != nil {
			return nil, err
		}
	}
	for id, source := range previous {
		if _, exists := current[id]; !exists {
			current[id] = inheritedProviderChannelDiscount(period, id, source, true)
		}
	}
	for _, id := range ids {
		if _, exists := current[id]; !exists {
			current[id] = inheritedProviderChannelDiscount(period, id, ProviderChannelBillingDiscount{}, false)
		}
	}
	return current, nil
}
