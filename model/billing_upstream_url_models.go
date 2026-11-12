package model

import (
	"context"
	"errors"
	"strings"
)

type UpstreamURLModelRow struct {
	ProviderURLChannelModelSummary
	ChannelID    int                                `json:"channel_id"`
	ChannelName  string                             `json:"channel_name"`
	UpstreamName string                             `json:"upstream_name,omitempty"`
	Deleted      bool                               `json:"deleted,omitempty"`
	Discount     *ProviderBillingDiscountProjection `json:"discount"`
}

func getUpstreamURLModelPage(ctx context.Context, start, end, period int64, filter UpstreamSummaryPageFilter, result UpstreamSummaryPage) (UpstreamSummaryPage, error) {
	report := BillingReportFromContext(ctx)
	if filter.ChannelID <= 0 {
		return result, errors.New("upstream model rows require a channel")
	}
	if report != nil && filter.ReportURL == "" {
		return result, errors.New("upstream daily report model rows require a URL")
	}
	if report == nil && filter.ReportURL != "" {
		return result, errors.New("upstream model rows filtered by URL require a daily report")
	}
	if filter.DataStatus != "" && filter.DataStatus != "complete" && filter.DataStatus != "partial" {
		return result, errors.New("invalid upstream data status")
	}
	var summary ProviderURLSummary
	var err error
	var names map[int]string
	if report != nil {
		summary, err = report.upstreamSummary(ctx, start, end, filter.ReportURL, filter.ChannelID)
	} else {
		summary, err = cachedUpstreamSummary(ctx, start, end, period)
		if err == nil {
			names, err = providerBillingChannelNames(ctx)
		}
	}
	if err != nil {
		return result, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	search := strings.ToLower(filter.Search)
	for _, group := range summary.Groups {
		for _, channel := range group.Channels {
			if channel.ChannelId != filter.ChannelID {
				continue
			}
			if names != nil {
				channel.UpstreamName = names[channel.ChannelId]
			}
			for _, leaf := range channel.Models {
				if search != "" && !strings.Contains(strings.ToLower(leaf.ProviderModel), search) {
					continue
				}
				complete := leaf.OriginalAmount != nil
				if (filter.DataStatus == "complete" && !complete) || (filter.DataStatus == "partial" && complete) {
					continue
				}
				if result.Total >= offset && len(result.ModelRows) < filter.PageSize {
					result.ModelRows = append(result.ModelRows, UpstreamURLModelRow{
						ProviderURLChannelModelSummary: leaf,
						ChannelID:                      channel.ChannelId, ChannelName: channel.ChannelName,
						UpstreamName: channel.UpstreamName, Deleted: group.Deleted,
						Discount: channel.Discount,
					})
				}
				result.Total++
			}
		}
	}
	return result, nil
}
