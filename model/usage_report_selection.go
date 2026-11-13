package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

type UsageReportRebuildStatus struct {
	RebuildingDays        []string `json:"rebuilding_days,omitempty"`
	SourceUnavailableDays []string `json:"source_unavailable_days,omitempty"`
}

// selectUsageReportDays is the common read boundary for summaries, ranks and
// exports. Older published days remain readable during and after rebuilding.
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
		if head.MeterVersion < usageReportMeterVersion && len(refs) == 0 {
			if issues[day.Start] {
				meta.SourceUnavailableDays = append(meta.SourceUnavailableDays, day.Date)
			} else {
				meta.RebuildingDays = append(meta.RebuildingDays, day.Date)
			}
		}
		if len(refs) == 0 && day.End <= now && head.AsOf < day.End+3*3600 {
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
	_, current, err := selectUsageReportDays(ctx, period, nil)
	if err != nil {
		return current, err
	}
	if len(refs) != 0 {
		frozen, err := FreezeUsageReportReferences(ctx, period, refs)
		if err != nil {
			return current, err
		}
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
		current.References = frozen
	}
	if len(current.References) == 0 {
		return current, common.NewMessage("Usage report has not been published")
	}
	return current, nil
}
