package model

import (
	"context"
	"time"
)

type dailyReportLogWindow struct {
	start int64
	upper int64
}

type dailyReportLogBounds struct {
	billingPrevious int64
	usagePrevious   int64
	includeUsage    bool
}

type dailyReportDateLogCoverage struct {
	BillingRows         int64
	BillingRevision     string
	BillingPreviousRows int64
	UsageRows           int64
	UsageRevision       string
	UsagePreviousRows   int64
}

func dailyReportLogTypes(kind string) []int {
	if kind == DailyReportUsage {
		return usageScanLogTypes(usageViewOptions{WantCustomer: true, WantUpstream: true})
	}
	return []int{LogTypeConsume, LogTypeRefund}
}

func (coverage dailyReportDateLogCoverage) forKind(kind string) (dailyReportCoverage, int64) {
	if kind == DailyReportUsage {
		return dailyReportCoverage{Rows: coverage.UsageRows, Revision: coverage.UsageRevision}, coverage.UsagePreviousRows
	}
	return dailyReportCoverage{Rows: coverage.BillingRows, Revision: coverage.BillingRevision}, coverage.BillingPreviousRows
}

// Each indexable timestamp range contributes to one statement snapshot. The
// scan includes the largest required boundary, while the filtered aggregates
// preserve independent current and previous cutoffs.
func readPostgresDailyReportLogCoverage(ctx context.Context, window dailyReportLogWindow, bounds dailyReportLogBounds) (dailyReportDateLogCoverage, error) {
	billingTypes := dailyReportLogTypes(DailyReportBilling)
	types := billingTypes
	if bounds.includeUsage {
		types = dailyReportLogTypes(DailyReportUsage)
	}
	var coverage dailyReportDateLogCoverage
	err := executeDailyReportBatch(ctx, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return usageReportLogDB(ctx).WithContext(ctx).Raw(`
SELECT CAST(COALESCE(SUM(part.rows) FILTER (WHERE part.type IN ?), 0) AS bigint) AS billing_rows,
       CAST(COALESCE(SUM(part.revision) FILTER (WHERE part.type IN ?), 0) AS text) AS billing_revision,
       CAST(COALESCE(SUM(part.billing_previous) FILTER (WHERE part.type IN ?), 0) AS bigint) AS billing_previous_rows,
       CAST(COALESCE(SUM(part.rows), 0) AS bigint) AS usage_rows,
       CAST(COALESCE(SUM(part.revision), 0) AS text) AS usage_revision,
       CAST(COALESCE(SUM(part.usage_previous), 0) AS bigint) AS usage_previous_rows
FROM generate_series(CAST(? AS bigint), CAST(? AS bigint), 3600) AS hours(start_at)
CROSS JOIN LATERAL (
    SELECT type,
           COUNT(*) FILTER (WHERE id <= ?) AS rows,
           SUM(`+dailyReportTupleRevision+`) FILTER (WHERE id <= ?) AS revision,
           COUNT(*) FILTER (WHERE id <= ?) AS billing_previous,
           COUNT(*) FILTER (WHERE id <= ?) AS usage_previous
    FROM logs
    WHERE created_at >= hours.start_at AND created_at < hours.start_at + 3600
      AND id <= ? AND type IN ?
    GROUP BY type
) AS part`, billingTypes, billingTypes, billingTypes, window.start, window.start+billingReportDaySeconds-3600,
			window.upper, window.upper, bounds.billingPrevious, bounds.usagePrevious,
			max(window.upper, bounds.billingPrevious, bounds.usagePrevious), types).Scan(&coverage).Error
	})
	return coverage, err
}

// Historical evidence recovery can check a smaller boundary and stop before
// scanning the full date when the original source is already complete.
func readDailyReportDateCoverage(ctx context.Context, start int64, kind string, upper int64) (dailyReportCoverage, error) {
	if err := usageReportBeforeBatch(ctx); err != nil {
		return dailyReportCoverage{}, err
	}
	if usageReportLogDB(ctx).Dialector.Name() == "postgres" {
		dates, err := readPostgresDailyReportLogCoverage(ctx, dailyReportLogWindow{start, upper},
			dailyReportLogBounds{includeUsage: kind == DailyReportUsage})
		coverage, _ := dates.forKind(kind)
		return coverage, err
	}
	return readDailyReportCoverage(ctx, dailyReportLogQuery(ctx, start, start+billingReportDaySeconds, upper, dailyReportLogTypes(kind)))
}
