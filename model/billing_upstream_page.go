package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type UpstreamSummaryPageFilter struct {
	Level          string
	ChannelID      int
	Search         string
	Page, PageSize int
}

// One channel level and one model level. URL membership is never a scope.
type UpstreamSummaryPage struct {
	Channels    []ProviderURLChannelGroupSummary  `json:"channels"`
	Models      []ProviderURLChannelModelSummary  `json:"models"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	Page        int                               `json:"page"`
	PageSize    int                               `json:"page_size"`
	Total       int                               `json:"total"`
	ModelCount  int64                             `json:"model_count"`
}

func GetUpstreamSummaryPage(ctx context.Context, start, end, period int64, filter UpstreamSummaryPageFilter) (UpstreamSummaryPage, error) {
	result := UpstreamSummaryPage{Channels: []ProviderURLChannelGroupSummary{}, Models: []ProviderURLChannelModelSummary{}, Page: filter.Page, PageSize: filter.PageSize}
	if filter.Page < 1 || filter.Page > 1000000 || filter.PageSize < 1 || filter.PageSize > 100 || filter.ChannelID < 0 {
		return result, errors.New("invalid pagination")
	}
	if filter.Level != "channels" && filter.Level != "models" && filter.Level != "options" {
		return result, errors.New("invalid summary level")
	}
	if filter.Level == "models" && filter.ChannelID <= 0 {
		return result, errors.New("models require channel_id")
	}
	names, err := providerBillingChannelNames(ctx)
	if err != nil {
		return result, err
	}
	var channels []ProviderURLChannelGroupSummary
	if filter.Level == "options" {
		ids, err := upstreamPeriodChannelIDs(ctx, start, end)
		if err != nil {
			return result, err
		}
		current, err := getBillingURLChannelsById(ids)
		if err != nil {
			return result, err
		}
		for _, id := range ids {
			channel, found := current[id]
			channels = append(channels, ProviderURLChannelGroupSummary{ChannelId: id, ChannelName: channel.Name, Deleted: !found, Models: []ProviderURLChannelModelSummary{}})
		}
	} else {
		// Reuse existing accounting leaves, never URL totals or URL names.
		summary, err := cachedUpstreamSummary(ctx, start, end, period)
		if err != nil {
			return result, err
		}
		for _, group := range summary.Groups {
			for _, channel := range group.Channels {
				channel.Deleted = group.Deleted
				channels = append(channels, channel)
			}
		}
	}
	sort.Slice(channels, func(i, j int) bool { return channels[i].ChannelId < channels[j].ChannelId })
	offset := (filter.Page - 1) * filter.PageSize
	search := strings.ToLower(filter.Search)
	for _, channel := range channels {
		channel.UpstreamName = names[channel.ChannelId]
		if filter.ChannelID > 0 && filter.ChannelID != channel.ChannelId {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%d %s %s", channel.ChannelId, channel.ChannelName, channel.UpstreamName)), search) {
			continue
		}
		identities := map[struct {
			Name     string
			Fallback bool
		}]bool{}
		for _, leaf := range channel.Models {
			identities[struct {
				Name     string
				Fallback bool
			}{leaf.ProviderModel, leaf.ProviderModelFallback}] = true
		}
		channel.ModelCount = int64(len(identities))
		result.ModelCount += channel.ModelCount
		accumulateBillingReconciliationQuality(&result.DataQuality, channel.DataQuality)
		if filter.Level == "models" {
			for _, leaf := range channel.Models {
				if result.Total >= offset && len(result.Models) < filter.PageSize {
					result.Models = append(result.Models, leaf)
				}
				result.Total++
			}
		} else {
			if result.Total >= offset && len(result.Channels) < filter.PageSize {
				channel.Models = []ProviderURLChannelModelSummary{}
				result.Channels = append(result.Channels, channel)
			}
			result.Total++
		}
	}
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return result, nil
}
