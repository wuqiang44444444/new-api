package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestErrorReportEnableAtBoundaryAndPausePreservesOutage(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, false, "ops@example.com")
	// Use the actual option persistence entry, not a pre-seeded cursor.
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", beijingTime(10, 59)))
	next, err := model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, beijingTime(10, 0).Unix(), next)
	// No runner processes 10-11 before disable; preserve that outage backlog.
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "false", beijingTime(11, 20)))
	next, err = model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Zero(t, next)
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", beijingTime(14, 20)))
	next, err = model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, beijingTime(10, 0).Unix(), next)
	_, err = model.AdvanceErrorReportWindow(next)
	require.NoError(t, err)
	next, err = model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, beijingTime(14, 0).Unix(), next, "explicitly disabled hours must be skipped")
}

func TestErrorReportOptionRejectsEmptyRecipientsWithoutChangingState(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	require.Error(t, model.UpdateOption("error_report_setting.recipients", ""))
	enabled, recipients, err := model.GetErrorReportConfiguration(context.Background())
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Equal(t, []string{"ops@example.com"}, recipients)
}

func TestErrorReportRemovedExpiredDeliveryIsNeverSent(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com;removed@example.com")
	withErrorReportSMTP(t)
	h := newErrorReportHandler(func(context.Context, string, string, string) error { return nil })
	h.now = func() time.Time { return beijingTime(11, 5) }
	require.NoError(t, h.processWindow(context.Background(), beijingTime(10, 0).Unix(), nil))
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).Where("recipient = ?", "removed@example.com").Updates(map[string]any{
		"status": model.ErrorReportDeliverySending, "lease_owner": "crashed", "lease_until": beijingTime(11, 0).Unix(),
	}).Error)
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	r := &errorReportMailRecorder{}
	h.send = r.send
	task := claimErrorReportTask(t, "runner")
	h.Run(context.Background(), task, "runner")
	require.NotEmpty(t, r.mails)
	for _, m := range r.mails {
		assert.Equal(t, "ops@example.com", m.rcpt)
	}
	var removed model.ErrorReportDelivery
	require.NoError(t, db.Where("recipient = ?", "removed@example.com").First(&removed).Error)
	assert.Equal(t, model.ErrorReportDeliveryStopped, removed.Status)
}

func TestErrorReportConfigChangeStopsFollowingSends(t *testing.T) {
	for _, change := range []string{"disable", "remove"} {
		t.Run(change, func(t *testing.T) {
			withErrorReportTestDB(t)
			withErrorReportSetting(t, true, "a@example.com;b@example.com")
			withErrorReportSMTP(t)
			calls := 0
			h := newErrorReportHandler(func(ctx context.Context, subject, recipient, body string) error {
				calls++
				if change == "disable" {
					return model.UpdateOption("error_report_setting.enabled", "false")
				}
				return model.UpdateOption("error_report_setting.recipients", recipient)
			})
			h.now = func() time.Time { return beijingTime(11, 5) }
			task := claimErrorReportTask(t, "runner")
			h.Run(context.Background(), task, "runner")
			assert.Equal(t, 1, calls)
		})
	}
}

func TestErrorReportDeliveryFailureRemainsVisibleAndDoesNotBlockOtherRecipients(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "a@example.com;b@example.com")
	withErrorReportSMTP(t)
	sent := []string{}
	h := newErrorReportHandler(func(ctx context.Context, subject, recipient, body string) error {
		if recipient == "a@example.com" {
			return errors.New("SMTP unavailable")
		}
		sent = append(sent, recipient)
		return nil
	})
	h.now = func() time.Time { return beijingTime(11, 5) }
	task := claimErrorReportTask(t, "runner")
	h.Run(context.Background(), task, "runner")
	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, []string{"b@example.com"}, sent)
	assert.Equal(t, model.SystemTaskStatusFailed, finished.Status)
	assert.Contains(t, finished.Error, "投递 1/2 成功")
	assert.Contains(t, finished.Error, "积压 1 封")
}

func TestErrorReportStreamAndProbeHeadlinesDoNotBlameCustomer(t *testing.T) {
	for _, tc := range []struct{ kind, reason, headline string }{
		{clienterrlog.EventStreamError, "upstream_response_failed", "发现平台／上游相关异常 / Platform / upstream errors detected"},
		{clienterrlog.EventStreamError, "stream_handler_panic", "发现平台／上游相关异常 / Platform / upstream errors detected"},
		{clienterrlog.EventStreamError, "stream_timeout", "发现流式或渠道测试异常 / Stream or channel test errors detected"},
		{clienterrlog.EventChannelTest, "unclassified", "发现流式或渠道测试异常 / Stream or channel test errors detected"},
	} {
		t.Run(tc.reason+tc.kind, func(t *testing.T) {
			event := &model.ErrorEvent{EventType: tc.kind, Reason: tc.reason, Status: 200}
			text := resolveErrorReportStatus(1, map[string]int{classifyErrorEvent(event): 1}, false, "")
			assert.Equal(t, tc.headline, text.Headline)
			assert.NotContains(t, text.Body, "未在本次数据中发现明确的平台／上游故障")
		})
	}
}

func TestErrorReportPublishedPartsCannotBeReplaced(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return beijingTime(11, 5) }
	start := beijingTime(10, 0).Unix()
	require.NoError(t, h.processWindow(context.Background(), start, nil))
	var parts []model.ErrorReportPart
	require.NoError(t, db.Find(&parts).Error)
	require.NotEmpty(t, parts)
	claims, err := model.ClaimErrorReportDeliveries(h.now().Unix(), "owner", h.now().Unix()+90, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	_, err = model.FinishErrorReportDelivery(claims[0].ID, "owner", model.ErrorReportDeliveryAccepted, "", "", 0, h.now().Unix())
	require.NoError(t, err)
	require.NoError(t, publishErrorReportTestParts(errorReportID(start), []*model.ErrorReportPart{{ReportID: errorReportID(start), PartNo: 1, BodyHTML: "replacement"}}, []string{"ops@example.com"}, 0, "{}", ""))
	var after []model.ErrorReportPart
	require.NoError(t, db.Find(&after).Error)
	assert.Equal(t, parts, after)
	var delivery model.ErrorReportDelivery
	require.NoError(t, db.First(&delivery, claims[0].ID).Error)
	assert.Equal(t, model.ErrorReportDeliveryAccepted, delivery.Status)
}

func TestErrorReportPerfWindowAndPublicCode(t *testing.T) {
	db := withErrorReportTestDB(t)
	start := beijingTime(10, 0).Unix()
	require.NoError(t, db.Create(&model.PerfMetric{ModelName: "test", BucketTs: start, RequestCount: 3, SuccessCount: 2}).Error)
	require.NoError(t, db.Create(&model.PerfMetric{ModelName: "test", BucketTs: start + 3600, RequestCount: 9, SuccessCount: 9}).Error)
	summaries, err := model.GetErrorReportPerfSummaries(context.Background(), start, start+3600)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, int64(3), summaries[0].RequestCount)
	h := newErrorReportHandler(nil)
	rows := h.renderDetailRow(1, &model.ErrorEvent{PublicCode: "insufficient_quota"}, errorReportLocation())
	assert.Contains(t, strings.Join(rows, ""), "insufficient_quota")
}

func TestErrorReportPublicationRejectsDisabledOrExcludedWindow(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "false", beijingTime(11, 20)))
	for _, tc := range []struct {
		start  int64
		reopen bool
	}{
		{beijingTime(10, 0).Unix(), false},
		{beijingTime(12, 0).Unix(), true},
	} {
		if tc.reopen {
			require.NoError(t, model.UpdateErrorReportOption("error_report_setting.enabled", "true", beijingTime(14, 20)))
		}
		id := errorReportID(tc.start)
		_, err := model.InsertErrorReportBuilding(&model.ErrorReport{ReportID: id, WindowStart: tc.start, WindowEnd: tc.start + 3600, Status: model.ErrorReportStatusBuilding})
		require.NoError(t, err)
		require.Error(t, publishErrorReportTestParts(id, []*model.ErrorReportPart{{ReportID: id, PartNo: 1, BodyHTML: "must not publish"}}, []string{"ops@example.com"}, 0, "{}", ""))
	}
	var count int64
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestErrorReportOptionWriteFailureRollsBackSchedule(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, false, "ops@example.com")
	before, err := model.GetErrorReportSchedule()
	require.NoError(t, err)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("report_option_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "options" {
			tx.AddError(errors.New("option write unavailable"))
		}
	}))
	defer db.Callback().Create().Remove("report_option_failure")
	require.Error(t, model.UpdateOption("error_report_setting.enabled", "true"))
	after, err := model.GetErrorReportSchedule()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	enabled, _, err := model.GetErrorReportConfiguration(context.Background())
	require.NoError(t, err)
	assert.False(t, enabled)
}
