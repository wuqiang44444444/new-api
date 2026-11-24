package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementExportCalendarDistinguishesCoverageAndEmptyDays(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	dayStart := func(offset int64) int64 { return start + offset*billingReportDaySeconds }
	date := func(offset int64) string {
		return time.Unix(dayStart(offset), 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	}
	for offset := int64(0); offset < 6; offset++ {
		at := dayStart(offset)
		row := billingReportGeneration{ID: fmt.Sprintf("%032x", offset+1), Start: at,
			AsOf: at + billingReportDaySeconds + dailyReportCloseDelay, Published: true, OriginalReplayVersion: CustomerOriginalReplayVersion}
		switch offset {
		case 3:
			row.OriginalReplayVersion--
		case 4:
			row.HistoricalSource = dailyReportSourceMissing
		case 5:
			row.AsOf--
		}
		require.NoError(t, db.Create(&row).Error)
		require.NoError(t, db.Create(&billingReportHead{Start: at, Generation: row.ID, AsOf: row.AsOf}).Error)
		if offset != 2 {
			require.NoError(t, db.Create(&billingReportCustomerDay{Generation: row.ID, UserID: 7}).Error)
		}
	}
	require.NoError(t, db.Create(&dailyReportRetry{Start: dayStart(1), Kind: DailyReportBilling, CoverageIssue: true,
		ReleaseVersion: CustomerOriginalReplayVersion}).Error)

	ctx, report, err := ResolveBillingReport(ctx, start, dayStart(7)-1, "")
	require.NoError(t, err)
	calendar, err := report.CustomerStatementExportCalendar(ctx, start, dayStart(7))
	require.NoError(t, err)
	assert.Equal(t, date(0), calendar.DefaultDate)
	assert.Equal(t, []CustomerStatementExportDay{
		{Date: date(6), Status: "unpublished"},
		{Date: date(5), Status: "unpublished"},
		{Date: date(4), Status: "source_unavailable"},
		{Date: date(3), Status: "rebuilding"},
		{Date: date(2), Status: "empty"},
		{Date: date(1), Status: "incomplete", ReportVersion: fmt.Sprintf("%032x", 2)},
		{Date: date(0), Status: "complete", ReportVersion: fmt.Sprintf("%032x", 1)},
	}, calendar.Days)
}
