package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type UpstreamSummaryPageFilter struct {
	GroupByURL     bool
	ReportURL      string
	Level          string
	ChannelID      int
	Search         string
	SortBy         string
	DataStatus     string
	Page, PageSize int
}

// Read-only upstream reconciliation projection.
type UpstreamSummaryPage struct {
	Totals      *UpstreamURLTotals                `json:"totals,omitempty"`
	ModelRows   []UpstreamURLModelRow             `json:"model_rows,omitempty"`
	URLChannels []UpstreamURLChannelRow           `json:"url_channels,omitempty"`
	URLGroups   []ProviderURLGroupSummary         `json:"url_groups,omitempty"`
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
	if filter.Level == "url_channels" {
		return getUpstreamURLChannelPage(ctx, start, end, period, filter, result)
	}
	if filter.Level == "url_models" {
		return getUpstreamURLModelPage(ctx, start, end, period, filter, result)
	}
	if filter.Level != "channels" && filter.Level != "models" && filter.Level != "options" {
		return result, errors.New("invalid summary level")
	}
	if filter.Level == "models" && filter.ChannelID <= 0 {
		return result, errors.New("models require channel_id")
	}
	if filter.GroupByURL && filter.Level == "channels" && filter.ChannelID == 0 {
		return getUpstreamURLGroupPage(ctx, start, end, period, filter, result)
	}
	report := BillingReportFromContext(ctx)
	var names map[int]string
	if report == nil {
		var err error
		names, err = providerBillingChannelNames(ctx)
		if err != nil {
			return result, err
		}
	}
	var channels []ProviderURLChannelGroupSummary
	if filter.Level == "options" && report == nil {
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
		var summary ProviderURLSummary
		var err error
		if report != nil {
			summary, err = report.upstreamSummary(ctx, start, end, filter.ReportURL, filter.ChannelID)
		} else {
			summary, err = cachedUpstreamSummary(ctx, start, end, period)
		}
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
		if report == nil {
			channel.UpstreamName = names[channel.ChannelId]
		}
		if filter.ChannelID > 0 && filter.ChannelID != channel.ChannelId {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%d %s %s", channel.ChannelId, channel.ChannelName, channel.UpstreamName)), search) {
			continue
		}
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
