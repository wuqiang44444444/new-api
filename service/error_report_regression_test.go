package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestErrorReportOptionRejectsInvalidRecipientsWithoutChangingState(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	require.Error(t, model.UpdateOption("error_report_setting.recipients", "invalid-address"))
	recipients, err := model.GetOperationsMailRecipients(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"ops@example.com"}, recipients)
}

func TestDailyReportResumesWindowAfterRecipientsReturn(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	ctx := context.Background()
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, errorReportLocation())
	start, err := model.DailyReportCursor(ctx, day.Unix())
	require.NoError(t, err)
	id := seedCompletedDailyWindow(t, ctx, start)
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", ""))
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return day.AddDate(0, 0, 3) }
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	assert.Zero(t, generated)
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	generated, _, err = h.generateDailyReport(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, generated)
	var report model.ErrorReport
	require.NoError(t, db.Where("report_id = ?", id).First(&report).Error)
	assert.Equal(t, model.ErrorReportStatusReady, report.Status)
}

func TestDailyReportDeliveryKnownFailureAndUnknown(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "retry@example.com;unknown@example.com;accepted@example.com;expired@example.com")
	ctx := context.Background()
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	now := day.AddDate(0, 0, 1).Add(6 * time.Hour)
	attempts := map[string]int{}
	h := newErrorReportHandler(func(_ context.Context, _, address, _ string) error {
		attempts[address]++
		if attempts[address] == 1 {
			switch address {
			case "retry@example.com":
				return errors.New("SMTP rejected the submission with status 421")
			case "unknown@example.com":
				return common.ErrMailSubmissionUnknown
			}
		}
		return nil
	})
	h.now = func() time.Time { return now }
	id := publishTestDailyWindow(t, h, day.Unix())
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).Where("recipient = ?", "expired@example.com").
		Updates(map[string]any{"status": model.MailDeliverySending, "lease_owner": "crashed", "lease_until": now.Unix() - 1}).Error)
	h.Run(ctx, claimErrorReportTask(t, "daily-runner"), "daily-runner")
	now = now.Add(time.Hour)
	attempted, accepted, err := h.deliverClaims(ctx, "later")
	require.NoError(t, err)
	assert.Equal(t, 1, attempted)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, map[string]int{"accepted@example.com": 1, "retry@example.com": 2, "unknown@example.com": 1}, attempts)
	var deliveries []model.ErrorReportDelivery
	require.NoError(t, db.Where("report_id = ?", id).Order("recipient").Find(&deliveries).Error)
	require.Len(t, deliveries, 4)
	assert.Equal(t, model.MailDeliveryAccepted, deliveries[0].Status)
	assert.Equal(t, model.MailDeliveryUnknown, deliveries[1].Status)
	assert.Equal(t, model.MailDeliveryAccepted, deliveries[2].Status)
	assert.Equal(t, model.MailDeliveryUnknown, deliveries[3].Status)
}

func TestDailyReportChecksConfigurationBetweenRecipients(t *testing.T) {
	for _, change := range []string{"clear", "remove"} {
		t.Run(change, func(t *testing.T) {
			withErrorReportTestDB(t)
			withErrorReportSetting(t, "a@example.com;b@example.com")
			day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
			calls := 0
			h := newErrorReportHandler(func(_ context.Context, _, address, _ string) error {
				calls++
				if change == "clear" {
					return model.UpdateOption("error_report_setting.recipients", "")
				}
				return model.UpdateOption("error_report_setting.recipients", address)
			})
			h.now = func() time.Time { return day.AddDate(0, 0, 1).Add(6 * time.Hour) }
			publishTestDailyWindow(t, h, day.Unix())
			_, _, err := h.deliverClaims(context.Background(), "daily-runner")
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
		})
	}
}

func TestDailyReportPublicationIsAtomicAndFrozen(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	ctx := context.Background()
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, errorReportLocation())
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return day.AddDate(0, 0, 1).Add(6 * time.Hour) }
	start, err := model.DailyReportCursor(ctx, day.Unix())
	require.NoError(t, err)
	id := seedCompletedDailyWindow(t, ctx, start)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test_daily_delivery_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "error_report_deliveries" {
			tx.AddError(errors.New("delivery write unavailable"))
		}
	}))
	_, _, err = h.generateDailyReport(ctx)
	require.Error(t, err)
	require.NoError(t, db.Callback().Create().Remove("test_daily_delivery_failure"))
	var parts []model.ErrorReportPart
	require.NoError(t, db.Where("report_id = ?", id).Find(&parts).Error)
	assert.Empty(t, parts)
	generated, _, err := h.generateDailyReport(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, generated)
	require.NoError(t, db.Where("report_id = ?", id).Find(&parts).Error)
	require.Len(t, parts, 1)
	require.NoError(t, model.PublishDailyReport(ctx, &model.ErrorReportPart{ReportID: id, PartNo: 1, BodyHTML: "replacement"}, []string{"ops@example.com"}, 0, "{}", ""))
	var frozen model.ErrorReportPart
	require.NoError(t, db.First(&frozen, parts[0].ID).Error)
	assert.Equal(t, parts[0], frozen)
}

func TestDailyReportRecipientFairnessAcrossDays(t *testing.T) {
	withErrorReportTestDB(t)
	withErrorReportSetting(t, "a@example.com;b@example.com")
	day := time.Date(2026, 11, 8, 0, 0, 0, 0, errorReportLocation())
	now := day.AddDate(0, 0, 2).Add(6 * time.Hour)
	h := newErrorReportHandler(nil)
	h.now = func() time.Time { return now }
	first := publishTestDailyWindow(t, h, day.Unix())
	second := publishTestDailyWindow(t, h, day.AddDate(0, 0, 1).Unix())
	claims, err := model.ClaimErrorReportDeliveries(now.Unix(), "first", now.Unix()+90, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	assert.Equal(t, "a@example.com", claims[0].Recipient)
	assert.Equal(t, first, claims[0].ReportID)
	ok, err := model.FinishErrorReportDelivery(claims[0].ID, "first", model.MailDeliveryRetryWait, "", "", now.Unix()+60, 0)
	require.NoError(t, err)
	require.True(t, ok)
	for _, id := range []string{first, second} {
		claims, err := model.ClaimErrorReportDeliveries(now.Unix(), "next", now.Unix()+90, 1)
		require.NoError(t, err)
		require.Len(t, claims, 1)
		assert.Equal(t, "b@example.com", claims[0].Recipient)
		assert.Equal(t, id, claims[0].ReportID)
		ok, err := model.FinishErrorReportDelivery(claims[0].ID, "next", model.MailDeliveryAccepted, "", "", 0, now.Unix())
		require.NoError(t, err)
		require.True(t, ok)
	}
	claims, err = model.ClaimErrorReportDeliveries(now.Unix(), "blocked", now.Unix()+90, 1)
	require.NoError(t, err)
	assert.Empty(t, claims)
}
