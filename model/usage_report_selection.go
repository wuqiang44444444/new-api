package model

import (
	"context"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type UsageReportRebuildStatus struct {
	RebuildingDays        []string `json:"rebuilding_days,omitempty"`
	SourceUnavailableDays []string `json:"source_unavailable_days,omitempty"`
}

// selectUsageReportDays is the common read boundary for summaries, ranks and
// exports. A pending upgrade withholds the previous day's numbers.
func selectUsageReportDays(ctx context.Context, period UsageAnalyticsPeriod, refs []UsageReportDayReference) ([]usageReportDay, PublishedUsageReport, error) {
	meta := PublishedUsageReport{PublishedDays: []string{}, References: []UsageReportDayReference{}}
	heads, err := reportDayHeads(ctx, period, refs)
	if err != nil {
		return nil, meta, err
	}
	var issues map[int64]bool
	if len(refs) == 0 && len(heads) > 0 {
		issues, err = dailyReportCoverageIssues(ctx, DailyReportUsage, period.StartTimestamp, period.EndTimestamp)
		if err != nil {
			return nil, meta, err
		}
	}
	byStart := make(map[int64]usageReportDay, len(heads))
	for _, head := range heads {
		byStart[head.Start] = head
	}
	now := common.GetTimestamp()
	selected := heads[:0]
	for _, day := range period.Days {
		head, ok := byStart[day.Start]
		if !ok || day.Future {
			continue
		}
		if head.MeterVersion != usageReportMeterVersion || head.HistoricalSource == dailyReportSourceMissing {
			if len(refs) != 0 {
				return nil, meta, common.NewMessage("Usage report version is unavailable")
			}
			if head.MeterVersion == usageReportMeterVersion && head.HistoricalSource == dailyReportSourceMissing {
				meta.SourceUnavailableDays = append(meta.SourceUnavailableDays, day.Date)
				meta.refundOnlyGenerations = append(meta.refundOnlyGenerations, head.Generation)
			} else {
				meta.RebuildingDays = append(meta.RebuildingDays, day.Date)
			}
			continue
		}
		if len(refs) == 0 && day.End <= now && head.AsOf < day.End+dailyReportCloseDelay {
			continue
		}
		if head.Incomplete || issues[day.Start] {
			meta.IncompleteDays = append(meta.IncompleteDays, day.Date)
		}
		selected = append(selected, head)
		meta.PublishedDays = append(meta.PublishedDays, day.Date)
		meta.References = append(meta.References, UsageReportDayReference{Start: day.Start, Generation: head.Generation})
		if meta.AsOf == 0 || head.AsOf < meta.AsOf {
			meta.AsOf = head.AsOf
		}
	}
	return selected, meta, nil
}

func FreezeUsageReportExport(ctx context.Context, period UsageAnalyticsPeriod, refs []UsageReportDayReference) (PublishedUsageReport, error) {
	heads, current, err := selectUsageReportDays(ctx, period, nil)
	if err != nil {
		return current, err
	}
	if len(refs) != 0 {
		selectedHeads, selected, err := selectUsageReportDays(ctx, period, refs)
		if err != nil {
			return current, err
		}
		frozen := selected.References
		// If a date was published after the page loaded, refresh the snapshot
		// before accepting an export that would silently omit it.
		if len(frozen) != len(current.References) {
			return current, common.NewMessage("Usage report version is unavailable")
		}
		for i, ref := range frozen {
			if ref.Start != current.References[i].Start {
				return current, common.NewMessage("Usage report version is unavailable")
			}
		}
		current.IncompleteDays = append(current.IncompleteDays, selected.IncompleteDays...)
		current.References = frozen
		heads = selectedHeads
	}
	if len(current.References) == 0 {
		return current, common.NewMessage("Usage report has not been published")
	}
	for _, head := range heads {
		if head.AsOf < head.Start+billingReportDaySeconds+dailyReportCloseDelay {
			current.IncompleteDays = append(current.IncompleteDays, time.Unix(head.Start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly))
		}
	}
	slices.Sort(current.IncompleteDays)
	current.IncompleteDays = slices.Compact(current.IncompleteDays)
	return current, nil
}
