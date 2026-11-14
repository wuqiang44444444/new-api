package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailPreferencesGateEveryMailTypeIndependently(t *testing.T) {
	for _, entry := range model.MailTypeCatalog() {
		t.Run(string(entry.ID), func(t *testing.T) {
			db := withErrorReportTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
			ctx := context.Background()
			_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{entry.ID: true}, nil)
			require.NoError(t, err)
			before := time.Now()
			var delivered int
			send := func(context.Context, string, string, string) error { delivered++; return nil }
			require.NoError(t, submitManagedMail(ctx, send, entry.ID, before, "fixture", "ops@example.com", "fixture"))
			_, err = model.UpdateMailPreferences(ctx, map[model.MailType]bool{entry.ID: false}, nil)
			require.NoError(t, err)
			require.ErrorIs(t, submitManagedMail(ctx, send, entry.ID, before, "fixture", "ops@example.com", "fixture"), errMailPreferenceDisabled)
			other := model.MailAccountSecurity
			if entry.ID == other {
				other = model.MailChannelBalance
			}
			require.NoError(t, submitManagedMail(ctx, send, other, time.Now(), "fixture", "ops@example.com", "fixture"))
			_, err = model.UpdateMailPreferences(ctx, map[model.MailType]bool{entry.ID: true}, nil)
			require.NoError(t, err)
			require.ErrorIs(t, submitManagedMail(ctx, send, entry.ID, before, "fixture", "ops@example.com", "fixture"), errMailPreferenceDisabled)
			require.NoError(t, submitManagedMail(ctx, send, entry.ID, time.Now(), "fixture", "ops@example.com", "fixture"))
			assert.Equal(t, 3, delivered)
		})
	}
}

func TestMailPreferencesStopsPendingRowsAndPreservesOperationalFacts(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}, &model.User{}, &model.WalletOperation{}))
	ctx := context.Background()
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	t.Setenv("FINANCE_MAIL_RECIPIENTS", "finance@example.com")
	inbox := newLocalMailInbox(t)
	now := time.Now()
	channel := model.Channel{Name: "mail-policy", Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	episode := func(key, kind string) model.ChannelAlertEpisode {
		return model.ChannelAlertEpisode{OpenKey: &key, ScopeKey: key, ChannelID: channel.Id, Revision: model.ChannelAlertRevision(&channel), Kind: kind, FirstAt: now.UnixMilli(), LastAt: now.UnixMilli()}
	}
	require.NoError(t, confirmChannelAlert(ctx, episode("balance", "upstream_balance"), now))
	require.NoError(t, confirmChannelAlert(ctx, episode("service", "upstream_service"), now))
	require.NoError(t, db.Create(&model.ErrorReportDelivery{Status: model.MailDeliveryRetryWait, Recipient: "ops@example.com", NextAttemptAt: now.Add(time.Hour).Unix()}).Error)
	user := model.User{Username: "mail-fixture", Email: "account@example.com", Quota: 100, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	_, err := model.AdjustUserQuotaWithMail(user.Id, common.RoleRootUser, "subtract", 100, 42, QueueWalletOperationMail)
	require.NoError(t, err)
	_, err = model.UpdateMailPreferences(ctx, map[model.MailType]bool{
		model.MailChannelBalance: false, model.MailDailyErrors: false, model.MailWalletReceipt: false, model.MailWalletRisk: false, model.MailCPU: false,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, confirmChannelAlert(ctx, episode("balance-2", "upstream_balance"), now))
	_, err = observeResourceAlert(ctx, "fixture", resourceObservation{"cpu", "CPU", 96, 85}, now.Unix())
	require.NoError(t, err)
	_, err = model.AdjustUserQuotaWithMail(user.Id, common.RoleRootUser, "add", 10, 42, QueueWalletOperationMail)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Model(&model.ChannelAlertEpisode{}).Count(&count).Error)
	assert.EqualValues(t, 3, count)
	require.NoError(t, db.Model(&model.ResourceAlertEpisode{}).Where("resource = ?", "cpu").Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.WalletOperation{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 10, user.Quota)
	var deliveries []model.ResourceAlertDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 2)
	for _, delivery := range deliveries {
		assert.Equal(t, model.MailDeliveryStopped, delivery.Status)
		assert.Equal(t, model.MailStopPreferenceDisabled, delivery.StopReason)
	}
	var daily model.ErrorReportDelivery
	require.NoError(t, db.First(&daily).Error)
	assert.Equal(t, model.MailStopPreferenceDisabled, daily.StopReason)
	sent, err := deliverChannelAlertMail("mail-policy", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	requireLocalMail(t, inbox, "Channel outage", "ops@example.com")
	sent, err = deliverChannelAlertMail("mail-policy", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	assert.False(t, sent)

	_, err = model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailChannelBalance: true}, nil)
	require.NoError(t, err)
	sent, err = deliverChannelAlertMail("mail-policy", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	requireLocalMail(t, inbox, "Upstream channel balance alert", "ops@example.com")
	sent, err = deliverChannelAlertMail("mail-policy", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	assert.False(t, sent, "the cancelled message was not requeued")
}

func TestMailPreferencesStopsClaimedMailBeforeTransport(t *testing.T) {
	withErrorReportTestDB(t)
	ctx := context.Background()
	queued := time.Now()
	send := func(ctx context.Context, subject, address, body string) error {
		_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailAccountSecurity: false}, nil)
		require.NoError(t, err)
		// Exercise the real SMTP submit guard. The local inbox must stay empty.
		return common.SendEmailContext(ctx, subject, address, body)
	}
	inbox := newLocalMailInbox(t)
	require.ErrorIs(t, submitManagedMail(ctx, send, model.MailAccountSecurity, queued, "fixture", "account@example.com", "fixture"), errMailPreferenceDisabled)
	assert.Empty(t, inbox)
}

func TestMailPreferencesUsesPersonalEmailAndKeepsAccountSecurityIndependent(t *testing.T) {
	oldCount, oldDuration := constant.NotifyLimitCount, constant.NotificationLimitDurationMinute
	constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = 2, 10
	t.Cleanup(func() { constant.NotifyLimitCount, constant.NotificationLimitDurationMinute = oldCount, oldDuration })
	withErrorReportTestDB(t)
	inbox := newLocalMailInbox(t)
	ctx := context.Background()
	_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailWalletLow: false}, nil)
	require.NoError(t, err)
	settings := dto.UserSetting{NotifyType: dto.NotifyTypeEmail, NotificationEmail: "personal@example.com"}
	require.NoError(t, notifyUser(model.MailWalletLow, time.Now(), 31, "account@example.com", settings, dto.NewNotify("wallet-fixture", "wallet fixture", "fixture", nil), false))
	require.NoError(t, notifyUser(model.MailSubscriptionLow, time.Now(), 31, "account@example.com", settings, dto.NewNotify("subscription-fixture", "subscription fixture", "fixture", nil), false))
	requireLocalMail(t, inbox, "subscription fixture", "personal@example.com")
	require.NoError(t, NotifyAccountSecurityChange("account@example.com", "Password updated"))
	requireLocalMail(t, inbox, "Account security", "account@example.com")
	assert.Empty(t, inbox)
}

func TestMailPreferencesMutedBalanceCheckStillObservesUpstream(t *testing.T) {
	setupUpstreamBalanceAlertTest(t)
	ctx := context.Background()
	_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailLowUpstreamBalance: false}, nil)
	require.NoError(t, err)
	h := upstreamBalanceAlertHandler{
		now: time.Now,
		query: func(context.Context, *model.Channel, int, string) UpstreamBalanceResult {
			return UpstreamBalanceResult{Status: "ok", Amounts: []UpstreamBalanceAmount{{Amount: "1", Unit: "USD"}}}
		},
		send: func(context.Context, string, string, string) error {
			t.Error("muted balance check submitted mail")
			return nil
		},
	}
	result, err := h.checkAndNotify(ctx, "fixture")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Checked)
	assert.Equal(t, 1, result.Low)
	assert.Zero(t, result.Sent)
}

func TestMailPreferencesMutedDailyErrorReportStillPublishes(t *testing.T) {
	db := withErrorReportTestDB(t)
	ctx := context.Background()
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailDailyErrors: false}, nil)
	require.NoError(t, err)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, errorReportLocation())
	h := newErrorReportHandler(func(context.Context, string, string, string) error {
		t.Error("muted daily report submitted mail")
		return nil
	})
	h.now = func() time.Time { return start.Add(31 * time.Hour) }
	id := publishTestDailyWindow(t, h, start.Unix())
	var report model.ErrorReport
	require.NoError(t, db.Where("report_id = ?", id).First(&report).Error)
	assert.Equal(t, model.ErrorReportStatusReady, report.Status)
	var count int64
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}
