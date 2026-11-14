package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

const (
	PersonalUsageReportMail = "personal_usage_report"
	PlatformUsageReportMail = "platform_usage_report"
)

// PublishedUsageMailDay shares the console's finalized-day and coverage rules.
func PublishedUsageMailDay(ctx context.Context, period UsageAnalyticsPeriod) (PublishedUsageReport, error) {
	_, meta, err := selectUsageReportDays(ctx, period, nil)
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
