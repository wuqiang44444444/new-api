package model

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
)

type UpstreamURLChannelRow struct {
	ProviderURLChannelGroupSummary
	URLKey  string `json:"url_key"`
	BaseURL string `json:"base_url,omitempty"`
	URLName string `json:"url_name"`
}

func getUpstreamURLChannelPage(ctx context.Context, start, end, period int64, filter UpstreamSummaryPageFilter, result UpstreamSummaryPage) (UpstreamSummaryPage, error) {
	if filter.SortBy != "" && filter.SortBy != "reference_amount" && filter.SortBy != "requests" {
		return result, errors.New("invalid upstream sort")
	}
	summary, err := cachedUpstreamSummary(ctx, start, end, period)
	if err != nil {
		return result, err
	}
	var names map[int]string
	if BillingReportFromContext(ctx) == nil {
		names, err = providerBillingChannelNames(ctx)
		if err != nil {
			return result, err
		}
	}
	search := strings.ToLower(filter.Search)
	totals := UpstreamURLTotals{}
	discountPending := make(map[int]struct{})
	var original, reference upstreamURLAmountTotal
	var rows []UpstreamURLChannelRow
	for _, group := range summary.Groups {
		urlMatches := search == "" || strings.Contains(strings.ToLower(group.DisplayName+" "+group.BaseURL), search)
		for _, channel := range group.Channels {
			if filter.ChannelID > 0 && filter.ChannelID != channel.ChannelId {
				continue
			}
			if names != nil {
				channel.UpstreamName = names[channel.ChannelId]
			}
			identity := strconv.Itoa(channel.ChannelId) + " " + channel.ChannelName + " " + channel.UpstreamName
			if !urlMatches && !strings.Contains(strings.ToLower(identity), search) {
				continue
			}
			channel.Deleted = group.Deleted
			channel.Models = nil
			rows = append(rows, UpstreamURLChannelRow{
				ProviderURLChannelGroupSummary: channel, URLKey: group.UrlKey,
				BaseURL: group.BaseURL, URLName: group.DisplayName,
			})
			accumulateProviderBillingUsage(&totals.Usage, channel.Usage)
			original.add(channel.OriginalAmount, channel.KnownOriginalAmount)
			reference.add(channel.ReferenceAmount, channel.KnownReferenceAmount)
			if channel.Discount == nil && !channel.UsageOnly {
				discountPending[channel.ChannelId] = struct{}{}
			}
			result.ModelCount += channel.ModelCount
			accumulateBillingReconciliationQuality(&result.DataQuality, channel.DataQuality)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		if filter.SortBy == "requests" {
			if left.Usage.Requests != right.Usage.Requests {
				return left.Usage.Requests > right.Usage.Requests
			}
		} else {
			a, b := left.ReferenceAmount, right.ReferenceAmount
			if (a == nil) != (b == nil) {
				return a != nil
			}
			if a != nil && *a != *b {
				return *a > *b
			}
		}
		if left.URLKey != right.URLKey {
			return left.URLKey < right.URLKey
		}
		return left.ChannelId < right.ChannelId
	})
	result.Total = len(rows)
	offset := (filter.Page - 1) * filter.PageSize
	if offset < result.Total {
		result.URLChannels = rows[offset:min(offset+filter.PageSize, result.Total)]
	}
	totals.OriginalAmount, totals.KnownOriginalAmount = original.amounts()
	totals.ReferenceAmount, totals.KnownReferenceAmount = reference.amounts()
	totals.DiscountPendingChannels = len(discountPending)
	result.Totals = &totals
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return result, nil
}
