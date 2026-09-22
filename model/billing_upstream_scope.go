package model

import (
	"context"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Read only channel identities from the period, in bounded keyset pages. No
// usage JSON, token replay, task evidence or money calculations are needed.
func upstreamPeriodChannelIDs(ctx context.Context, start, end int64) ([]int, error) {
	var ids []int
	cursor := -1
	for {
		var batch []int
		err := LOG_DB.WithContext(ctx).Model(&Log{}).
			Where("type IN ? AND created_at >= ? AND created_at <= ? AND channel_id > ?", []int{LogTypeConsume, LogTypeRefund}, start, end, cursor).
			Distinct("channel_id").Order("channel_id ASC").Limit(100).Pluck("channel_id", &batch).Error
		if err != nil {
			return nil, err
		}
		ids = append(ids, batch...)
		if len(batch) < 100 {
			return ids, nil
		}
		cursor = batch[len(batch)-1]
	}
}

func upstreamURLOptions(ctx context.Context, start, end int64) (ProviderURLSummary, error) {
	result := ProviderURLSummary{Groups: []ProviderURLGroupSummary{}}
	ids, err := upstreamPeriodChannelIDs(ctx, start, end)
	if err != nil {
		return result, err
	}
	channels, err := getBillingURLChannelsById(ids)
	if err != nil {
		return result, err
	}
	names, err := getProviderURLGroupNames()
	if err != nil {
		return result, err
	}
	groups := map[string]ProviderURLGroupSummary{}
	for _, id := range ids {
		channel, found := channels[id]
		key, name := billingURLGroupIdentity(channel, found, id)
		group := groups[key]
		group.UrlKey, group.DisplayName, group.CustomName = key, name, names[key]
		group.Unidentified = strings.HasPrefix(key, billingURLGroupChannelFallbackPrefix)
		group.Deleted = group.Unidentified && id > 0 && !found
		if group.Unidentified {
			group.ChannelIds = []int{id}
		} else {
			group.BaseURL = key
		}
		group.ChannelCount++
		groups[key] = group
	}
	for _, group := range groups {
		result.Groups = append(result.Groups, group)
	}
	sort.Slice(result.Groups, func(i, j int) bool { return providerURLGroupLess(result.Groups[i], result.Groups[j]) })
	return result, nil
}

// The whole period is initialized independently of UI pagination. Each batch
// uses the same authoritative, audited and idempotent channel-month writer.
func InitializeUpstreamPeriodDiscounts(ctx context.Context, start, end int64, actor int) (map[string]int, error) {
	var actorUser User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actorUser, actor).Error; err != nil {
		return nil, err
	}
	if actorUser.Status != common.UserStatusEnabled || actorUser.Role < common.RoleAdminUser {
		return nil, ErrCustomerExportNotFound
	}
	counts := map[string]int{}
	ids, err := upstreamPeriodChannelIDs(ctx, start, end)
	if err != nil {
		return nil, err
	}
	for offset := 0; offset < len(ids); offset += 100 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outcomes, err := InitializeProviderChannelBillingDiscounts(start, ids[offset:min(offset+100, len(ids))], actor)
		if err != nil {
			return nil, err
		}
		for _, outcome := range outcomes {
			counts[outcome.Outcome]++
		}
	}
	return counts, nil
}

func providerURLGroupLess(left, right ProviderURLGroupSummary) bool {
	if left.Unidentified != right.Unidentified {
		return !left.Unidentified
	}
	a, b := left.DisplayName, right.DisplayName
	if left.CustomName != "" {
		a = left.CustomName
	}
	if right.CustomName != "" {
		b = right.CustomName
	}
	if a != b {
		return a < b
	}
	return left.UrlKey < right.UrlKey
}

// Metadata is deliberately selected without credentials and compared across
// nodes. A changed name, URL or monthly discount changes the read-cache key.
func upstreamSummaryMetadata(ctx context.Context, period int64) ([]byte, error) {
	var channels []Channel
	var batch []Channel
	err := DB.WithContext(ctx).Select("id, name, type, base_url").FindInBatches(&batch, 500, func(tx *gorm.DB, _ int) error { channels = append(channels, batch...); return nil }).Error
	if err != nil {
		return nil, err
	}
	discounts, err := loadProviderChannelBillingDiscounts(ctx, period, nil)
	if err != nil {
		return nil, err
	}
	names, err := getProviderURLGroupNames()
	if err != nil {
		return nil, err
	}
	return common.Marshal(struct {
		Channels  []Channel
		Discounts map[int]ProviderChannelBillingDiscount
		Names     map[string]string
	}{channels, discounts, names})
}
