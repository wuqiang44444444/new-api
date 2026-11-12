package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportCompletesPreviousBeijingDay(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	inbox := newLocalMailInbox(t)
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	dayStart := day.Unix()
	dayEnd := day.AddDate(0, 0, 1).Unix()
	reportID := fmt.Sprintf("daily-%d", dayStart)
	ctx := context.Background()

	require.NoError(t, db.Create(&[]model.ErrorEvent{
		{CreatedAt: dayStart - 1, EventType: "outside"},
		{CreatedAt: dayStart, EventType: "upstream_failure", ChannelId: 42, Detail: `{"http_exchange":{"request":{"body":"private diagnostic must not appear"}}}`},
		{CreatedAt: dayEnd - 1, EventType: "upstream_failure", ChannelId: 42},
		{CreatedAt: dayEnd, EventType: "outside"},
	}).Error)
	require.NoError(t, db.Create(&model.SystemTask{
		TaskID: "failed-ops-fixture", Type: "channel_test", Status: model.SystemTaskStatusFailed, UpdatedAt: dayEnd - 30,
	}).Error)
	require.NoError(t, db.Create(&model.ResourceAlertEpisode{Resource: "cpu", OpenedAt: dayEnd - 30}).Error)

	now := day.AddDate(0, 0, 1).Add(5*time.Hour + 59*time.Minute)
	h := newErrorReportHandler(common.SendEmailContext)
	h.now = func() time.Time { return now }
	require.True(t, h.Enabled())
	report, err := model.LoadDailyReport(ctx, dayStart, reportID)
	require.NoError(t, err)

	// One run records partial progress. A source outage cannot advance it.
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	assert.Zero(t, generated)
	require.NoError(t, db.Where("report_id = ?", reportID).First(report).Error)
	progress := report.AggregatedThrough
	assert.Greater(t, progress, dayStart)
	assert.Less(t, progress, dayEnd)
	logDB := model.LOG_DB
	model.LOG_DB = nil
	_, _, err = h.generateDailyReport(ctx)
	model.LOG_DB = logDB
	require.Error(t, err)
	require.NoError(t, db.Where("report_id = ?", reportID).First(report).Error)
	assert.Equal(t, progress, report.AggregatedThrough)

	// Finish every slice before 06:00. No report can be delivered yet.
	for report.AggregatedThrough < dayEnd {
		previous := report.AggregatedThrough
		generated, _, err = h.generateDailyReport(ctx)
		require.NoError(t, err)
		require.Zero(t, generated)
		require.NoError(t, db.Where("report_id = ?", reportID).First(report).Error)
		require.Greater(t, report.AggregatedThrough, previous)
	}
	attempted, _, err := h.deliverClaims(ctx, "before-six")
	require.NoError(t, err)
	assert.Zero(t, attempted)

	now = now.Add(time.Minute)
	generated, _, err = h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	_, accepted, err := h.deliverClaims(ctx, "at-six")
	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	mail := requireLocalMail(t, inbox, "Daily operations report", "ops@example.com")
	assert.Contains(t, mail.subject, "2026-11-09")
	for _, text := range []string{"<strong>2</strong>", "upstream_failure", "#42", "channel_test", "cpu"} {
		assert.Contains(t, mail.body, text)
	}
	assert.NotContains(t, mail.body, "private diagnostic")
	assert.NotContains(t, mail.body, "outside")

	attempted, _, err = h.deliverClaims(ctx, "restart")
	require.NoError(t, err)
	assert.Zero(t, attempted)
}

func TestDailyReportWaitsForLateAggregationEvenAfterSix(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	start := day.Unix()
	end := day.AddDate(0, 0, 1).Unix()
	now := day.Add(31 * time.Hour)
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return now }
	ctx := context.Background()
	report, err := model.LoadDailyReport(ctx, start, fmt.Sprintf("daily-%d", start))
	require.NoError(t, err)
	require.NoError(t, db.Model(report).Update("aggregated_through", end-35*60).Error)
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Zero(t, generated)
	generated, _, err = h.generateDailyReport(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, generated)
	require.NoError(t, db.Where("report_id = ?", report.ReportID).First(report).Error)
	assert.Equal(t, model.ErrorReportStatusReady, report.Status)
	assert.Zero(t, report.TotalEvents)
}
