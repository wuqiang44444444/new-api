package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

func getUpstreamURLGroupPage(ctx context.Context, start, end, period int64, filter UpstreamSummaryPageFilter, result UpstreamSummaryPage) (UpstreamSummaryPage, error) {
	summary, err := cachedUpstreamSummary(ctx, start, end, period)
	if err != nil {
		return result, err
	}
	report := BillingReportFromContext(ctx)
	var names map[int]string
	if report == nil {
		names, err = providerBillingChannelNames(ctx)
		if err != nil {
			return result, err
		}
	}
	switch filter.SortBy {
	case "", "reference_amount":
		sort.SliceStable(summary.Groups, func(i, j int) bool {
			a, b := summary.Groups[i].ReferenceAmount, summary.Groups[j].ReferenceAmount
			return a != nil && (b == nil || *a > *b)
		})
	case "requests":
		sort.SliceStable(summary.Groups, func(i, j int) bool {
			return summary.Groups[i].Usage.Requests > summary.Groups[j].Usage.Requests
		})
	default:
		return result, errors.New("invalid upstream sort")
	}
	totals := UpstreamURLTotals{}
	var original, reference upstreamURLAmountTotal
	search := strings.ToLower(filter.Search)
	offset := (filter.Page - 1) * filter.PageSize
	for _, group := range summary.Groups {
		matches := search == "" || strings.Contains(strings.ToLower(group.DisplayName+" "+group.BaseURL), search)
		for _, channel := range group.Channels {
			name := channel.UpstreamName
			if report == nil {
				name = names[channel.ChannelId]
			}
			if !matches && strings.Contains(strings.ToLower(fmt.Sprintf("%d %s %s", channel.ChannelId, channel.ChannelName, name)), search) {
				matches = true
			}
		}
		if !matches {
			continue
		}
		if result.Total >= offset && len(result.URLGroups) < filter.PageSize {
			// The URL header retains the accounting totals computed by the
			// shared reader. Only the channel's model leaves load on demand.
			sort.Slice(group.Channels, func(i, j int) bool { return group.Channels[i].ChannelId < group.Channels[j].ChannelId })
			for i := range group.Channels {
				channel := &group.Channels[i]
				if report == nil {
					channel.UpstreamName = names[channel.ChannelId]
				}
				channel.Deleted = group.Deleted
				channel.Models = []ProviderURLChannelModelSummary{}
			}
			result.URLGroups = append(result.URLGroups, group)
		}
		accumulateProviderBillingUsage(&totals.Usage, group.Usage)
		original.add(group.OriginalAmount, group.KnownOriginalAmount)
		reference.add(group.ReferenceAmount, group.KnownReferenceAmount)
		result.Total++
		result.ModelCount += group.ModelCount
		accumulateBillingReconciliationQuality(&result.DataQuality, group.DataQuality)
	}
	totals.OriginalAmount, totals.KnownOriginalAmount = original.amounts()
	totals.ReferenceAmount, totals.KnownReferenceAmount = reference.amounts()
	result.Totals = &totals
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return result, nil
}

type UpstreamURLTotals struct {
	Usage                ProviderBillingUsage `json:"usage"`
	OriginalAmount       *int64               `json:"original_amount,omitempty"`
	KnownOriginalAmount  *int64               `json:"known_original_amount,omitempty"`
	ReferenceAmount      *int64               `json:"reference_amount,omitempty"`
	KnownReferenceAmount *int64               `json:"known_reference_amount,omitempty"`
}

type upstreamURLAmountTotal struct {
	known      decimal.Decimal
	hasKnown   bool
	incomplete bool
}

func (total *upstreamURLAmountTotal) add(amount, known *int64) {
	if amount == nil {
		total.incomplete = true
	} else {
		known = amount
	}
	if known != nil {
		total.known = total.known.Add(decimal.NewFromInt(*known))
		total.hasKnown = true
	}
}

func (total upstreamURLAmountTotal) amounts() (complete, known *int64) {
	if total.hasKnown {
		known = billingStatementOriginalQuota(total.known)
	}
	if !total.incomplete {
		complete = known
	}
	return complete, known
}
