package model

import (
	"context"
	"errors"
	"strings"
)

// Each request returns one level only. Totals describe the selected scope,
// never the current page; child lists are fetched separately on expansion.
type UpstreamSummaryPageFilter struct {
	Level     string
	URLKey    string
	ChannelID int
	Search    string
	Page      int
	PageSize  int
}

type UpstreamSummaryPage struct {
	Groups      []ProviderURLGroupSummary         `json:"url_groups"`
	Channels    []ProviderURLChannelGroupSummary  `json:"channels"`
	Models      []ProviderURLChannelModelSummary  `json:"models"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	Page        int                               `json:"page"`
	PageSize    int                               `json:"page_size"`
	Total       int                               `json:"total"`
	ModelCount  int64                             `json:"model_count"`
}

func GetUpstreamSummaryPage(ctx context.Context, start, end, period int64, filter UpstreamSummaryPageFilter) (UpstreamSummaryPage, error) {
	result := UpstreamSummaryPage{Groups: []ProviderURLGroupSummary{}, Channels: []ProviderURLChannelGroupSummary{}, Models: []ProviderURLChannelModelSummary{}, Page: filter.Page, PageSize: filter.PageSize}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 || filter.Page > 1000000 {
		return result, errors.New("invalid pagination")
	}
	switch filter.Level {
	case "groups", "options":
		if filter.ChannelID != 0 {
			return result, errors.New("channel_id requires models level")
		}
	case "channels":
		if filter.URLKey == "" || filter.ChannelID != 0 {
			return result, errors.New("channels require url_key")
		}
	case "models":
		if filter.URLKey == "" || filter.ChannelID <= 0 {
			return result, errors.New("models require url_key and channel_id")
		}
	default:
		return result, errors.New("invalid summary level")
	}
	var summary ProviderURLSummary
	var err error
	if filter.Level == "options" {
		summary, err = upstreamURLOptions(ctx, start, end)
	} else {
		summary, err = cachedUpstreamSummary(ctx, start, end, period)
	}
	if err != nil {
		return result, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	for _, group := range summary.Groups {
		if filter.URLKey != "" && group.UrlKey != filter.URLKey {
			continue
		}
		if filter.Search != "" && !strings.Contains(strings.ToLower(group.DisplayName+" "+group.CustomName+" "+group.UrlKey), strings.ToLower(filter.Search)) {
			continue
		}
		if filter.Level != "models" {
			result.ModelCount += group.ModelCount
			accumulateBillingReconciliationQuality(&result.DataQuality, group.DataQuality)
		}
		switch filter.Level {
		case "groups", "options":
			if result.Total >= offset && len(result.Groups) < filter.PageSize {
				group.UsageOnly = len(group.Channels) > 0
				for _, channel := range group.Channels {
					group.UsageOnly = group.UsageOnly && channel.UsageOnly
				}
				group.Channels = []ProviderURLChannelGroupSummary{}
				if !group.Unidentified {
					group.ChannelIds = []int{}
				}
				result.Groups = append(result.Groups, group)
			}
			result.Total++
		case "channels":
			for _, channel := range group.Channels {
				if result.Total >= offset && len(result.Channels) < filter.PageSize {
					channel.Models = []ProviderURLChannelModelSummary{}
					result.Channels = append(result.Channels, channel)
				}
				result.Total++
			}
		case "models":
			for _, channel := range group.Channels {
				if channel.ChannelId != filter.ChannelID {
					continue
				}
				accumulateBillingReconciliationQuality(&result.DataQuality, channel.DataQuality)
				identities := make(map[struct {
					name     string
					fallback bool
				}]struct{})
				for _, leaf := range channel.Models {
					identities[struct {
						name     string
						fallback bool
					}{leaf.ProviderModel, leaf.ProviderModelFallback}] = struct{}{}
					if result.Total >= offset && len(result.Models) < filter.PageSize {
						result.Models = append(result.Models, leaf)
					}
					result.Total++
				}
				result.ModelCount += int64(len(identities))
			}
		}
	}
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return result, nil
}
