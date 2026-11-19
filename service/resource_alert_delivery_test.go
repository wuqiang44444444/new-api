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
)

func TestResourceAlertRetriesOnlyDefinitelyUnsubmittedRecipients(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "retry@example.com;unknown@example.com;accepted@example.com")
	now := time.Date(2026, 11, 10, 7, 0, 0, 0, time.UTC)
	ctx := context.Background()
	sample := resourceObservation{key: "memory", label: "Memory", usage: 85, threshold: 85}
	observed, err := observeResourceAlert(ctx, "node", sample, now.Unix())
	require.NoError(t, err)
	require.True(t, observed)
	// No in-process job has been scheduled. A fresh delivery worker can recover
	// all the pending work from the database after queue saturation or restart.
	attempts := map[string]int{}
	send := func(_ context.Context, _, recipient, body string) error {
		assert.Contains(t, body, "85.0%")
		attempts[recipient]++
		if attempts[recipient] == 1 {
			switch recipient {
			case "retry@example.com":
				return errors.New("SMTP rejected the submission with status 421")
			case "unknown@example.com":
				return common.ErrMailSubmissionUnknown
			}
		}
		return nil
	}
	clock := func() time.Time { return now }
	require.NoError(t, deliverResourceAlerts(clock, send))
	observed, err = observeResourceAlert(ctx, "node", sample, now.Unix()+15)
	require.NoError(t, err)
	require.True(t, observed)
	now = now.Add(time.Minute)
	require.NoError(t, deliverResourceAlerts(clock, send))
	assert.Equal(t, map[string]int{"retry@example.com": 2, "unknown@example.com": 1, "accepted@example.com": 1}, attempts)
	var count int64
	require.NoError(t, db.Model(&model.ResourceAlertEpisode{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var uncertain model.ResourceAlertDelivery
	require.NoError(t, db.Where("recipient = ?", "unknown@example.com").Take(&uncertain).Error)
	assert.Equal(t, model.MailDeliveryUnknown, uncertain.Status)
}

func TestImmediateMailIsClaimedAheadOfUsageBacklog(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, "ops@example.com")
	t.Setenv("FINANCE_MAIL_RECIPIENTS", "finance@example.com")
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Task{}, &model.TaskBillingDelivery{}))
	prepareDailyReportExportTest(t)
	period, err := model.ResolveUsageAnalyticsPeriod("day", now.In(model.UsageAnalyticsZone()).AddDate(0, 0, -2).Format(time.DateOnly), now.Unix())
	require.NoError(t, err)
	require.NoError(t, model.BuildUsageReportDay(ctx, period.Date, "export-fixture", "runner", nil))
	report, err := model.PublishedUsageMailDay(ctx, period)
	require.NoError(t, err)
	require.Len(t, report.References, 1)
	ref := report.References[0]
	for _, kind := range []string{model.PlatformUsageReportMail, model.CustomerStatusReportMail} {
		for i := range 3 {
			generation := ref.Generation
			if i == 2 {
				generation = ""
			}
			opened, err := model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
				Resource: kind, OpenedAt: ref.Start, Subject: kind, UsageReportGeneration: generation,
			}, []string{"ops@example.com"})
			require.NoError(t, err)
			require.True(t, opened)
		}
	}
	require.NoError(t, recordEvidenceCleanupHealth(ctx, now.Unix(), &model.EvidenceCleanupHealth{Alerts: []string{"cleanup_stalled"}}, false))
	opened, err := model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		Resource: model.WalletRiskMail, OpenedAt: now.Unix(), Subject: "wallet-risk",
	}, []string{"finance@example.com"})
	require.NoError(t, err)
	require.True(t, opened)

	var sent []string
	require.NoError(t, deliverResourceAlerts(time.Now, func(_ context.Context, subject, _, _ string) error {
		sent = append(sent, subject)
		return nil
	}))
	require.Len(t, sent, 4)
	assert.Contains(t, sent[0], "Evidence cleanup alert")
	assert.Equal(t, "wallet-risk", sent[1])
	assert.Equal(t, []string{model.PlatformUsageReportMail, model.PlatformUsageReportMail}, sent[2:])
	require.NoError(t, deliverResourceAlerts(time.Now, func(_ context.Context, subject, _, _ string) error {
		sent = append(sent, subject)
		return nil
	}))
	assert.Equal(t, []string{model.CustomerStatusReportMail, model.CustomerStatusReportMail}, sent[4:])
	var held int64
	require.NoError(t, model.DB.Model(&model.ResourceAlertDelivery{}).
		Where("status = ? AND stop_reason = ? AND next_attempt_at > ?", model.MailDeliveryRetryWait, "report_version_unavailable", now.Unix()).Count(&held).Error)
	assert.EqualValues(t, 2, held)
}
