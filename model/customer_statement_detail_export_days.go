package model

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type CustomerStatementDetailExportDay struct {
	Date           string `json:"date"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"` // exclusive
	Incomplete     bool   `json:"incomplete,omitempty"`
}

func (report *BillingReportManifest) CustomerStatementDetailExportDays(ctx context.Context, userID int) ([]CustomerStatementDetailExportDay, error) {
	days := make([]CustomerStatementDetailExportDay, 0, len(report.generations))
	if len(report.generations) == 0 {
		return days, nil
	}
	db := DB.WithContext(ctx)
	customerLines := db.Model(&billingReportLine{}).Select("1").
		Where("billing_report_lines.generation = billing_report_generations.id AND user_id = ? AND fund_group <> ?", userID, "")
	var generations []string
	if err := db.Model(&billingReportGeneration{}).Where("id IN ?", report.ids()).
		Where("EXISTS (?)", customerLines).Pluck("id", &generations).Error; err != nil {
		return nil, err
	}
	populated := make(map[string]bool, len(generations))
	for _, id := range generations {
		populated[id] = true
	}
	now := common.GetTimestamp()
	for i := len(report.generations) - 1; i >= 0; i-- {
		row := report.generations[i]
		if !populated[row.ID] || row.Start > now {
			continue
		}
		end := row.Start + billingReportDaySeconds
		days = append(days, CustomerStatementDetailExportDay{
			Date: time.Unix(row.Start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly), StartTimestamp: row.Start, EndTimestamp: end,
			Incomplete: row.Incomplete || row.AsOf < end+dailyReportCloseDelay,
		})
	}
	return days, nil
}

// A frozen month can contain millions of lines. Check each date using the
// version/time index and return only the date boundaries with a matching line.
func CustomerStatementVersionDetailExportDays(ctx context.Context, version *BillingStatementVersion, start, end int64) ([]CustomerStatementDetailExportDay, error) {
	db := DB.WithContext(ctx)
	var selects []string
	var args []any
	now := common.GetTimestamp()
	for day := start; day < end && day <= now; day += billingReportDaySeconds {
		lines := db.Model(&BillingStatementVersionLine{}).Select("created_at").
			Where("version_id = ? AND created_at >= ? AND created_at < ?", version.ID, day, day+billingReportDaySeconds).
			Order("created_at ASC").Limit(1)
		selects = append(selects, "SELECT created_at FROM (?) AS billing_day")
		args = append(args, lines)
	}
	var starts []int64
	if len(selects) > 0 {
		if err := db.Raw(strings.Join(selects, " UNION ALL ")+" ORDER BY created_at DESC", args...).Scan(&starts).Error; err != nil {
			return nil, err
		}
	}
	days := make([]CustomerStatementDetailExportDay, 0, len(starts))
	for _, at := range starts {
		day := start + (at-start)/billingReportDaySeconds*billingReportDaySeconds
		days = append(days, CustomerStatementDetailExportDay{
			Date: time.Unix(day, 0).In(UsageAnalyticsZone()).Format(time.DateOnly), StartTimestamp: day, EndTimestamp: day + billingReportDaySeconds,
		})
	}
	return days, nil
}
