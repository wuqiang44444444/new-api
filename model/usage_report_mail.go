package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	PersonalUsageReportMail = "personal_usage_report"
	PlatformUsageReportMail = "platform_usage_report"
)

// PublishedUsageMailDay shares the console's finalized-day and coverage rules.
func PublishedUsageMailDay(ctx context.Context, period UsageAnalyticsPeriod) (PublishedUsageReport, error) {
	heads, meta, err := selectUsageReportDays(ctx, period, nil)
	if err == nil && len(heads) == 1 && heads[0].HistoricalSource != "" {
		// Frozen summaries describe an earlier meter. Leave the date's mail key
		// available for a subsequent rebuild from primary evidence.
		meta.References = nil
	}
	return meta, err
}

type UsageReportMailAccount struct {
	UserID int
	Email  string
	Total  UsageAnalyticsMetrics
}

// PublishedUsageMailAccounts reads only the frozen customer overview and the
// current account address. Pending-refund-only rows have no usage overview.
func PublishedUsageMailAccounts(ctx context.Context, day UsageReportDayReference, limit int) ([]UsageReportMailAccount, error) {
	var rows []struct {
		UserID   int
		Email    string
		Overview BillingStatementJSON
	}
	queued := DB.Model(&ResourceAlertEpisode{}).Select("mail_user_id").Where("opened_at = ? AND resource = ?", day.Start, PersonalUsageReportMail)
	err := DB.WithContext(ctx).Table("usage_report_day_rows AS report").
		Select("report.user_id, account.email, report.overview").
		Joins("JOIN users AS account ON account.id = report.user_id").
		Where("report.generation = ? AND report.user_id > 0 AND report.overview <> ''", day.Generation).
		Where("account.deleted_at IS NULL AND account.status = ? AND account.email <> ''", common.UserStatusEnabled).
		Where("report.user_id NOT IN (?)", queued).Order("report.user_id").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	accounts := make([]UsageReportMailAccount, 0, len(rows))
	for _, row := range rows {
		var projection usageReportMetrics
		if err := common.UnmarshalJsonStr(string(row.Overview), &projection); err != nil {
			return nil, err
		}
		accounts = append(accounts, UsageReportMailAccount{UserID: row.UserID, Email: row.Email, Total: projection.accumulator().finalize()})
	}
	return accounts, nil
}

// PublishedUsageMailTotals streams the same overview projections as the admin
// console, using its accumulator without the interactive customer-list limit.
func PublishedUsageMailTotals(ctx context.Context, day UsageReportDayReference) (UsageAnalyticsMetrics, int64, error) {
	rows, err := DB.WithContext(ctx).Model(&usageReportDayRow{}).Select("overview").
		Where("generation = ? AND user_id > 0 AND overview <> ''", day.Generation).Rows()
	if err != nil {
		return UsageAnalyticsMetrics{}, 0, err
	}
	defer rows.Close()
	var totals usageMetricsAcc
	var active int64
	for rows.Next() {
		var raw string
		var projection usageReportMetrics
		if err := rows.Scan(&raw); err != nil {
			return UsageAnalyticsMetrics{}, 0, err
		}
		if err := common.UnmarshalJsonStr(raw, &projection); err != nil {
			return UsageAnalyticsMetrics{}, 0, err
		}
		active++
		totals.merge(projection.accumulator())
	}
	return totals.finalize(), active, rows.Err()
}

// Check the generation that produced the outbox body. An old pending mail
// must never be treated as a newly calculated body after a version change.
func UsageReportMailVersionCurrent(ctx context.Context, episode ResourceAlertEpisode) (bool, error) {
	if episode.UsageReportGeneration == "" {
		return false, nil
	}
	var marker usageReportDayRow
	err := DB.WithContext(ctx).Select("payload").Where(
		"generation = ? AND user_id = -1 AND COALESCE(unavailable_at, 0) = 0", episode.UsageReportGeneration).Take(&marker).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var head usageReportDay
	if err := common.UnmarshalJsonStr(string(marker.Payload), &head); err != nil {
		return false, err
	}
	return head.Start == episode.OpenedAt && head.Generation == episode.UsageReportGeneration &&
		head.MeterVersion == usageReportMeterVersion && head.HistoricalSource == "" &&
		head.AsOf >= head.Start+billingReportDaySeconds+int64((3*time.Hour).Seconds()), nil
}
