package model

import (
	"context"
	"sort"

	"gorm.io/gorm"
)

// Freeze configuration only. Global evidence jobs discover log rows in the
// admitted executor; deleted channels remain covered by the frozen ID boundary.
func freezeUpstreamExportChannels(ctx context.Context, period int64, ids []int, allChannels bool) (map[int]UpstreamExportChannel, error) {
	result := make(map[int]UpstreamExportChannel)
	query := DB.WithContext(ctx).Model(&Channel{}).Select("id, name")
	if allChannels {
		var rows []Channel
		if err := query.FindInBatches(&rows, 100, func(_ *gorm.DB, _ int) error {
			for _, row := range rows {
				result[row.Id] = UpstreamExportChannel{Name: row.Name}
			}
			return nil
		}).Error; err != nil {
			return nil, err
		}
		// A deleted channel can still have an authoritative month coefficient.
		var discounts []ProviderChannelBillingDiscount
		if err := DB.WithContext(ctx).Select("id, channel_id").Where("period_start IN ?", []int64{period, previousBillingPeriodStart(period)}).FindInBatches(&discounts, 100, func(_ *gorm.DB, _ int) error {
			for _, row := range discounts {
				if _, exists := result[row.ChannelId]; !exists {
					result[row.ChannelId] = UpstreamExportChannel{}
				}
			}
			return nil
		}).Error; err != nil {
			return nil, err
		}
		ids = make([]int, 0, len(result))
		for id := range result {
			ids = append(ids, id)
		}
		sort.Ints(ids)
	} else {
		for offset := 0; offset < len(ids); offset += 100 {
			var rows []Channel
			if err := query.Session(&gorm.Session{}).Where("id IN ?", ids[offset:min(offset+100, len(ids))]).Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, row := range rows {
				result[row.Id] = UpstreamExportChannel{Name: row.Name}
			}
		}
	}
	discounts, err := loadProviderChannelBillingDiscounts(ctx, period, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		channel := result[id]
		if record, exists := discounts[id]; exists && record.PendingReason == "" {
			projection := providerChannelDiscountProjection(record)
			channel.Discount = &projection
		}
		result[id] = channel
	}
	return result, nil
}

// An absent global snapshot entry means neither a channel nor a month record
// existed at submission. Apply the same recorded default without reading live
// configuration. An existing entry with nil Discount remains explicitly unknown.
func (scope *UpstreamExportScope) Channel(id int, period int64) UpstreamExportChannel {
	channel, exists := scope.Channels[id]
	if !exists && scope.AllChannels {
		record := inheritedProviderChannelDiscount(period, id, ProviderChannelBillingDiscount{}, false)
		projection := providerChannelDiscountProjection(record)
		channel.Discount = &projection
	}
	return channel
}
