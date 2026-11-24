package model

import (
	"context"
	"slices"
	"time"
)

type CustomerStatementExportDay struct {
	Date          string `json:"date"`
	Status        string `json:"status"`
	ReportVersion string `json:"report_version,omitempty"`
}

type CustomerStatementExportCalendar struct {
	Days        []CustomerStatementExportDay `json:"days"`
	DefaultDate string                       `json:"default_date,omitempty"`
}

// CustomerStatementExportCalendar reads availability from the same published
// generations used for export. A published day can have upstream data only.
func (report *BillingReportManifest) CustomerStatementExportCalendar(ctx context.Context, start, endExclusive int64) (CustomerStatementExportCalendar, error) {
	calendar := CustomerStatementExportCalendar{Days: make([]CustomerStatementExportDay, 0)}
	generations := make(map[int64]billingReportGeneration, len(report.generations))
	for _, row := range report.generations {
		generations[row.Start] = row
	}
	var populated []string
	if len(generations) > 0 {
		db := DB.WithContext(ctx)
		customerRows := db.Model(&billingReportCustomerDay{}).Select("1").
			Where("billing_report_customer_days.generation = billing_report_generations.id")
		if err := db.Model(&billingReportGeneration{}).Where("id IN ?", report.ids()).
			Where("EXISTS (?)", customerRows).Pluck("id", &populated).Error; err != nil {
			return calendar, err
		}
	}

	zone := UsageAnalyticsZone()
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone).Unix()
	for at := min(endExclusive-billingReportDaySeconds, today); at >= start; at -= billingReportDaySeconds {
		date := time.Unix(at, 0).In(zone).Format(time.DateOnly)
		day := CustomerStatementExportDay{Date: date, Status: "unpublished"}
		row, published := generations[at]
		switch {
		case at == today:
			day.Status = "in_progress"
		case slices.Contains(report.RebuildingDays, date):
			day.Status = "rebuilding"
		case slices.Contains(report.SourceUnavailableDays, date):
			day.Status = "source_unavailable"
		case published && row.AsOf >= at+billingReportDaySeconds+dailyReportCloseDelay:
			hasRecords := slices.Contains(populated, row.ID)
			switch {
			case row.Incomplete:
				day.Status = "incomplete"
			case !hasRecords:
				day.Status = "empty"
			default:
				day.Status = "complete"
				if calendar.DefaultDate == "" {
					calendar.DefaultDate = date
				}
			}
			if hasRecords {
				day.ReportVersion = row.ID
			}
		}
		calendar.Days = append(calendar.Days, day)
	}
	return calendar, nil
}
