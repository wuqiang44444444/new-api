package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportPublishesHighCardinalityMultibyteWindow(t *testing.T) {
	db := withErrorReportTestDB(t)
	if dialect := os.Getenv("DAILY_REPORT_TEST_DIALECT"); dialect != "" {
		db = channelAlertDialectDB(t, dialect)
		previous := model.LOG_DB
		model.LOG_DB = db
		t.Cleanup(func() { model.LOG_DB = previous })
		require.NoError(t, db.AutoMigrate(&model.Option{}, &model.ErrorReportSchedule{}, &model.ErrorReport{},
			&model.ErrorReportPart{}, &model.ErrorReportDelivery{}, &model.ErrorEvent{}, &model.Channel{},
			&model.ChannelAlertEpisode{}, &model.ResourceAlertEpisode{}, &model.SystemTask{}))
	}
	withErrorReportSetting(t, "ops@example.com")
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://console.example.test"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })
	ctx := context.Background()
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	end := day.AddDate(0, 0, 1).Unix()
	report := seedCompletedDailyWindow(t, ctx, day.Unix())
	require.NoError(t, db.Model(&model.ErrorReport{}).Where("report_id = ?", report).
		UpdateColumn("aggregated_through", end-300).Error)

	categories := []struct {
		event, reason, signal string
		status                int
	}{
		{clienterrlog.EventAPIError, "", model.DailyErrorAPI5xx, 503},
		{clienterrlog.EventAPIError, "", model.DailyErrorAPI429, 429},
		{clienterrlog.EventAPIError, "", model.DailyErrorAPI4xx, 400},
		{clienterrlog.EventAPIError, clienterrlog.WalletQuotaInsufficient, clienterrlog.WalletQuotaInsufficient, 403},
		{clienterrlog.EventAPIError, clienterrlog.SubscriptionQuotaInsufficient, clienterrlog.SubscriptionQuotaInsufficient, 403},
		{clienterrlog.EventAPIError, clienterrlog.APIKeyQuotaInsufficient, clienterrlog.APIKeyQuotaInsufficient, 403},
		{clienterrlog.EventStreamError, "", clienterrlog.EventStreamError, 200},
		{clienterrlog.EventStreamError, "client_disconnected", model.DailyErrorStreamDisconnected, 200},
		{clienterrlog.EventTaskFailure, "", clienterrlog.EventTaskFailure, 0},
		{clienterrlog.EventVideoDelivery, "video_delivery_retry_exhausted", clienterrlog.EventVideoDelivery, 0},
	}
	var rows []model.ErrorEvent
	for _, category := range categories {
		for id := 1; id <= 12; id++ {
			rows = append(rows, model.ErrorEvent{
				CreatedAt: end - 1, EventType: category.event, Status: category.status,
				Reason: category.reason, ModelName: strings.Repeat("😀", 128), ChannelId: id,
			})
		}
	}
	require.NoError(t, db.Create(&rows).Error)
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return day.Add(30 * time.Hour) }
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)

	var saved model.ErrorReport
	require.NoError(t, db.Where("report_id = ?", report).Take(&saved).Error)
	assert.Equal(t, model.ErrorReportStatusReady, saved.Status)
	assert.Equal(t, len(rows), saved.TotalEvents)
	assert.LessOrEqual(t, len(saved.Summary), 65535)
	var summary dailyReportSummary
	require.NoError(t, common.UnmarshalJsonStr(saved.Summary, &summary))
	assert.True(t, summary.Approximate)
	for _, category := range categories {
		assert.EqualValues(t, 12, summary.Signals[category.signal], category.signal)
	}
	var part model.ErrorReportPart
	require.NoError(t, db.Where("report_id = ?", report).Take(&part).Error)
	assert.LessOrEqual(t, len(part.BodyHTML), 65535)
	assert.Contains(t, part.BodyHTML, "Recorded error breakdown")
	cursor, err := model.DailyReportCursor(ctx, day.Unix())
	require.NoError(t, err)
	assert.Equal(t, end, cursor)
}

func TestDailyReportOmitsPassedFoundryProbeAndKeepsOrdinaryFailures(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	ctx := t.Context()
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	end := day.AddDate(0, 0, 1).Unix()
	reportID := seedCompletedDailyWindow(t, ctx, day.Unix())
	require.NoError(t, db.Model(&model.ErrorReport{}).Where("report_id = ?", reportID).
		UpdateColumn("aggregated_through", end-300).Error)
	ordinary := model.ErrorEvent{CreatedAt: end - 2, EventType: clienterrlog.EventChannelTest,
		ChannelId: 59, Status: 200, PublicCode: "555420", Reason: "test_upstream_rejected",
		ModelName: "ordinary-probe-model", RequestId: "ordinary-probe",
		Detail: `{"test_mode":"auto","check_result":"failed","upstream_status":"400"}`}
	passed := ordinary
	passed.CreatedAt, passed.ModelName, passed.RequestId = end-1, "foundry-probe-model", "passed-probe"
	passed.Detail = `{"test_mode":"auto","check_result":"passed","generation_evidence":"rejected","upstream_status":"400"}`
	require.NoError(t, db.Create(&[]model.ErrorEvent{ordinary, passed}).Error)

	handler := newErrorReportHandler(nil)
	handler.now = func() time.Time { return day.Add(30 * time.Hour) }
	generated, _, err := handler.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	var saved model.ErrorReport
	require.NoError(t, db.Where("report_id = ?", reportID).Take(&saved).Error)
	assert.Equal(t, 1, saved.TotalEvents)
	var summary dailyReportSummary
	require.NoError(t, common.UnmarshalJsonStr(saved.Summary, &summary))
	assert.EqualValues(t, 1, summary.Signals[clienterrlog.EventChannelTest])
	require.Len(t, summary.Groups, 1)
	assert.Equal(t, ordinary.ModelName, summary.Groups[0].ModelName)
	var part model.ErrorReportPart
	require.NoError(t, db.Where("report_id = ?", reportID).Take(&part).Error)
	assert.Contains(t, part.BodyHTML, ordinary.ModelName)
	assert.NotContains(t, part.BodyHTML, passed.ModelName)
	rows, total, err := model.GetErrorEvents(model.ErrorEventFilter{EventType: clienterrlog.EventChannelTest, ChannelId: 59}, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, rows, 2)
	assert.Equal(t, passed.RequestId, rows[0].RequestId)
	assert.Equal(t, passed.Detail, rows[0].Detail)
}

func TestDailyReportCompletesPreviousBeijingDay(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	inbox := newLocalMailInbox(t)
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://console.example.test"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })
	require.NoError(t, db.Create(&model.Channel{Id: 42, Name: "Production <A>"}).Error)
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	dayStart := day.Unix()
	dayEnd := day.AddDate(0, 0, 1).Unix()
	reportID := fmt.Sprintf("daily-%d", dayStart)
	ctx := context.Background()

	require.NoError(t, db.Create(&[]model.ErrorEvent{
		{CreatedAt: dayStart - 1, EventType: "outside"},
		{CreatedAt: dayStart, EventType: clienterrlog.EventAPIError, Status: 503, Reason: "upstream_unavailable", ModelName: "image-v2", ChannelId: 42, Detail: `{"http_exchange":{"request":{"body":"private diagnostic must not appear"}}}`},
		{CreatedAt: dayEnd - 1, EventType: clienterrlog.EventChannelTest, Status: 200, Reason: "test_config_error", ModelName: "image-v2", ChannelId: 42},
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
	for _, text := range []string{
		"Recorded errors", "Customer API 5xx", "Probe and manual test failures", "upstream_unavailable", "test_config_error",
		"#42 Production &lt;A&gt;", "channel_test", "cpu", "Customer failures were also recorded for this channel and model.",
		fmt.Sprintf("https://console.example.test/usage-logs/error?channel=42&amp;end_timestamp=%d&amp;event_type=api_error&amp;model_name=%%22image-v2%%22&amp;reason=%%22upstream_unavailable%%22&amp;start_timestamp=%d&amp;status=503", dayEnd-1, dayStart),
	} {
		assert.Contains(t, mail.body, text)
	}
	assert.NotContains(t, mail.body, "private diagnostic")
	assert.NotContains(t, mail.body, "outside")
	assert.NotContains(t, mail.body, "HTTP 200")

	attempted, _, err = h.deliverClaims(ctx, "restart")
	require.NoError(t, err)
	assert.Zero(t, attempted)
}

func TestDailyReportDistinguishesConfirmedIncidentsFromProbeAndStreamNoise(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	start, end := day.Unix(), day.AddDate(0, 0, 1).Unix()
	ctx := context.Background()
	id := fmt.Sprintf("daily-%d", start)
	report, err := model.LoadDailyReport(ctx, start, id)
	require.NoError(t, err)
	require.NoError(t, model.SaveDailyReportProgress(ctx, report, end-300, `{"signals":{}}`, 0))
	require.NoError(t, db.Create(&[]model.ErrorEvent{
		{CreatedAt: end - 30, EventType: clienterrlog.EventChannelTest, Status: 200, Reason: "test_config_error", ChannelId: 3},
		{CreatedAt: end - 20, EventType: clienterrlog.EventStreamError, Status: 200, Reason: "client_disconnected", ChannelId: 3},
		{CreatedAt: end - 10, EventType: clienterrlog.EventStreamError, Status: 200, Reason: "stream_soft_errors", ChannelId: 3},
	}).Error)

	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return day.Add(30 * time.Hour) }
	require.NoError(t, db.Create(&[]model.ChannelAlertEpisode{
		{ChannelID: 42, Kind: "upstream_service", ConfirmedAt: (start - 60) * 1000, EvidenceCount: 3},
		{ChannelID: 43, Kind: "upstream_service", ConfirmedAt: (end - 60) * 1000, ClosedAt: (end + 60) * 1000, CloseReason: model.ChannelAlertClosedRecovered, EvidenceCount: 3},
		{ChannelID: 44, ConfirmedAt: (start - 60) * 1000, ClosedAt: start * 1000},
		{ChannelID: 45, ConfirmedAt: end * 1000},
		{ChannelID: 46, ConfirmedAt: (end - 60) * 1000, Preview: true},
	}).Error)
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	var part model.ErrorReportPart
	require.NoError(t, db.Where("report_id = ?", id).First(&part).Error)
	assert.Contains(t, part.BodyHTML, "Confirmed channel incidents · 2")
	assert.Contains(t, part.BodyHTML, "#42")
	assert.Contains(t, part.BodyHTML, "Still open")
	assert.Contains(t, part.BodyHTML, "#43")
	assert.Contains(t, part.BodyHTML, "Recovery observed")
	for _, excluded := range []string{"#44", "#45", "#46"} {
		assert.NotContains(t, part.BodyHTML, excluded)
	}
	assert.Contains(t, part.BodyHTML, "流式取消或客户端断开 / Stream cancellations or client disconnects</h4>")
	assert.Contains(t, part.BodyHTML, "流式软错误 / Stream warnings</h4>")
	assert.NotContains(t, part.BodyHTML, "需排查的流式异常 / Stream errors to investigate</h4>")
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
	require.NoError(t, db.Model(report).Updates(map[string]any{"aggregated_through": end - 35*60, "summary": `{"signals":{}}`}).Error)
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

func TestDailyReportBackfillDoesNotShowPrunedLogsAsZero(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial=%t", partial), func(t *testing.T) {
			db := withErrorReportTestDB(t)
			withErrorReportSetting(t, "ops@example.com")
			inbox := newLocalMailInbox(t)
			day := time.Date(2026, 10, 5, 0, 0, 0, 0, errorReportLocation())
			start, end := day.Unix(), day.AddDate(0, 0, 1).Unix()
			now := day.AddDate(0, 0, 35)
			cutoff := end
			if partial {
				cutoff = end - 300
			}
			ctx := context.Background()
			_, err := model.DailyReportCursor(ctx, start)
			require.NoError(t, err)
			require.NoError(t, db.Create(&[]model.ErrorEvent{
				{CreatedAt: cutoff - 1, EventType: clienterrlog.EventAPIError, Status: 503},
				{CreatedAt: cutoff, EventType: clienterrlog.EventAPIError, Status: 503, Reason: "retained_fixture"},
			}).Error)
			retention := errorEventRetentionHandler{}
			task, err := model.CreateSystemTask(retention.Type(), map[string]int64{"cutoff": cutoff}, nil)
			require.NoError(t, err)
			claimed, ok, err := model.ClaimSystemTask(task.ID, retention.Type(), "retention", common.GetTimestamp()+60)
			require.NoError(t, err)
			require.True(t, ok)
			retention.Run(ctx, claimed, "retention")
			finished, err := model.GetSystemTaskByTaskID(claimed.TaskID)
			require.NoError(t, err)
			require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)

			// Disabling retention does not restore previously deleted records.
			t.Setenv("ERROR_EVENTS_RETENTION_DAYS", "0")
			h := newErrorReportHandler(common.SendEmailContext)
			h.now = func() time.Time { return now }
			generated, _, err := h.generateDailyReport(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, generated)
			_, accepted, err := h.deliverClaims(ctx, "backfill")
			require.NoError(t, err)
			require.Equal(t, 1, accepted)
			mail := requireLocalMail(t, inbox, "2026-10-05", "ops@example.com")
			assert.Contains(t, mail.body, "Incomplete error log coverage")
			assert.Contains(t, mail.body, "The total error count is unknown")
			assert.NotContains(t, mail.body, "No errors or confirmed channel incidents")
			if partial {
				assert.Contains(t, mail.body, "<strong>≥1</strong>")
				assert.Contains(t, mail.body, "retained_fixture")
			} else {
				assert.Contains(t, mail.body, "<strong>未知 / Unknown</strong>")
				assert.NotContains(t, mail.body, "retained_fixture")
			}
			report, err := model.LoadDailyReport(ctx, start, fmt.Sprintf("daily-%d", start))
			require.NoError(t, err)
			assert.Contains(t, report.DataNote, "coverage incomplete")
		})
	}
}
