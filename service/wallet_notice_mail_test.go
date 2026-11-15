package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletRejectionDigestGroupsCustomersAndHonorsMailPreference(t *testing.T) {
	db := withErrorReportTestDB(t)
	ctx := context.Background()
	inbox := newLocalMailInbox(t)
	withErrorReportSetting(t, "ops@example.com")
	oldURL := system_setting.ServerAddress
	system_setting.ServerAddress = "https://gateway.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = oldURL })
	now := time.Now().In(errorReportLocation()).Truncate(time.Second).Add(2 * time.Hour)
	end := now.Truncate(walletRejectionWindow)
	if end.Hour() == 0 && end.Minute() < 30 {
		end = end.Add(time.Hour)
	}
	add := func(id int, at time.Time, reason string) {
		t.Helper()
		require.NoError(t, db.Create(&model.ErrorEvent{UserId: id, EventType: clienterrlog.EventAPIError,
			CreatedAt: at.Unix(), Status: 403, Reason: reason, RequestId: fmt.Sprintf("%d-%d", id, at.Unix())}).Error)
	}
	add(70, end.Add(-12*time.Minute), clienterrlog.WalletQuotaInsufficient)
	add(70, end.Add(-2*time.Minute), clienterrlog.WalletQuotaInsufficient)
	add(71, end.Add(-3*time.Minute), clienterrlog.WalletQuotaInsufficient)
	add(71, end.Add(-2*time.Minute), clienterrlog.WalletQuotaInsufficient)
	add(72, end.Add(-time.Minute), clienterrlog.WalletQuotaInsufficient)
	add(73, end.Add(-time.Minute), clienterrlog.SubscriptionQuotaInsufficient)
	tick := end.Add(time.Minute)

	require.NoError(t, queueWalletRejectionDigest(ctx, tick))
	var count int64
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.Zero(t, count, "disabled on upgrade")

	_, err := model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailWalletRejection: true}, nil)
	require.NoError(t, err)
	require.NoError(t, queueWalletRejectionDigest(ctx, tick.Add(-walletRejectionWindow)))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	prior := requireLocalMail(t, inbox, "Requests rejected due to insufficient wallet balance", "ops@example.com")
	assert.Contains(t, prior.body, "(#70)")
	require.NoError(t, queueWalletRejectionDigest(ctx, tick))
	require.NoError(t, queueWalletRejectionDigest(ctx, tick.Add(time.Minute)))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	mail := requireLocalMail(t, inbox, "Requests rejected due to insufficient wallet balance", "ops@example.com")
	assert.Contains(t, mail.body, "<!doctype html>")
	assert.Contains(t, mail.body, "(#71)")
	assert.NotContains(t, mail.body, "(#70)")
	assert.NotContains(t, mail.body, "(#73)")
	assert.Contains(t, mail.body, "/usage-logs/error?")
	assert.Contains(t, mail.body, clienterrlog.WalletQuotaInsufficient)
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 2, count)

	add(74, end.Add(4*time.Minute), clienterrlog.WalletQuotaInsufficient)
	require.NoError(t, queueWalletRejectionDigest(ctx, tick.Add(walletRejectionWindow)))
	_, err = model.UpdateMailPreferences(ctx, map[model.MailType]bool{model.MailWalletRejection: false}, nil)
	require.NoError(t, err)
	var stopped int64
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Where("status = ? AND stop_reason = ?", model.MailDeliveryStopped, model.MailStopPreferenceDisabled).Count(&stopped).Error)
	assert.EqualValues(t, 1, stopped)
}

func TestPersonalWalletMailPersistsOneNoticeUntilObservedRecovery(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	inbox := newLocalMailInbox(t)
	ctx := context.Background()
	at := time.Now().Add(-time.Minute)
	account := model.User{Id: 80, Username: "wallet", AffCode: "wallet80", Email: "account@example.com", Status: common.UserStatusEnabled, Quota: 50}
	account.SetSetting(dto.UserSetting{Language: "en", NotificationEmail: "personal@example.com", QuotaWarningThreshold: 100})
	require.NoError(t, db.Create(&account).Error)

	queue := func(when time.Time, rejected bool) bool {
		t.Helper()
		queued, err := queueWalletNotice(ctx, account.Id, when, rejected, nil)
		require.NoError(t, err)
		return queued
	}
	require.True(t, queue(at, true))
	require.False(t, queue(at.Add(time.Second), true))
	require.False(t, queue(at.Add(time.Second), false))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	first := requireLocalMail(t, inbox, "A request was declined", "personal@example.com")
	assert.Contains(t, first.body, "<!doctype html>")
	assert.Contains(t, first.body, "/wallet")

	require.NoError(t, db.Model(&account).Update("quota", 100).Error)
	h := walletNoticeHandler{}
	require.NoError(t, h.recoverWalletNotices(ctx, at.Add(10*time.Second)))
	require.NoError(t, db.Model(&account).Update("quota", 90).Error)
	require.True(t, queue(at.Add(20*time.Second), false))
	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	second := requireLocalMail(t, inbox, "wallet balance is low", "personal@example.com")
	assert.Contains(t, second.body, "warning threshold")
}

func TestPersonalWalletRejectionRemindsCustomerWithPositiveBalance(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	inbox := newLocalMailInbox(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 15, 23, 55, 0, 0, errorReportLocation())
	account := model.User{Id: 81, Username: "positive-wallet", AffCode: "wallet81", Email: "customer@example.com", Status: common.UserStatusEnabled, Quota: 120}
	account.SetSetting(dto.UserSetting{Language: "en", QuotaWarningThreshold: 100})
	require.NoError(t, db.Create(&account).Error)

	queued, err := queueWalletNotice(ctx, account.Id, at, false, nil)
	require.NoError(t, err)
	require.False(t, queued, "balance above the threshold does not trigger a low-balance notice")
	queued, err = queueWalletNotice(ctx, account.Id, at, true, nil)
	require.NoError(t, err)
	require.True(t, queued)
	queued, err = queueWalletNotice(ctx, account.Id, at.Add(10*time.Minute), true, nil)
	require.NoError(t, err)
	require.False(t, queued, "crossing midnight does not end the cooldown")

	require.NoError(t, deliverResourceAlerts(time.Now, common.SendEmailContext))
	mail := requireLocalMail(t, inbox, "A request was declined", "customer@example.com")
	assert.Contains(t, mail.body, "Your wallet could not cover a recent request")
	assert.Contains(t, mail.body, "positive-wallet (#81)")
}

func TestWalletDigestIncludesAnAccountNextWindowWhenItsFirstLogArrivedLate(t *testing.T) {
	db := withErrorReportTestDB(t)
	ctx := context.Background()
	withErrorReportSetting(t, "ops@example.com")
	require.NoError(t, db.Create(&model.Option{Key: "mail_preferences", Value: `{"types":{"customer_wallet_rejection":{"enabled":true}}}`}).Error)
	end := time.Date(2026, 11, 15, 17, 30, 0, 0, errorReportLocation())
	record := func(id int, at time.Time) {
		t.Helper()
		require.NoError(t, db.Create(&model.ErrorEvent{UserId: id, EventType: clienterrlog.EventAPIError, Status: 403,
			Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: at.Unix()}).Error)
	}
	record(101, end.Add(-4*time.Minute))
	require.NoError(t, queueWalletRejectionDigest(ctx, end.Add(time.Minute)))
	record(102, end.Add(-time.Minute))
	record(102, end.Add(4*time.Minute))
	record(101, end.Add(5*time.Minute))
	require.NoError(t, queueWalletRejectionDigest(ctx, end.Add(2*time.Minute)))
	require.NoError(t, queueWalletRejectionDigest(ctx, end.Add(11*time.Minute)))
	var episodes []model.ResourceAlertEpisode
	require.NoError(t, db.Where("resource = ?", model.CustomerWalletRejectionMail).Order("opened_at").Find(&episodes).Error)
	require.Len(t, episodes, 2)
	assert.Contains(t, episodes[0].BodyHTML, "(#101)")
	assert.Contains(t, episodes[1].BodyHTML, "(#102)")
	assert.NotContains(t, episodes[1].BodyHTML, "(#101)")
}

func TestWalletDeliveryRechecksBalanceButKeepsConfirmedRejection(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	ctx := context.Background()
	for _, id := range []int{91, 92} {
		account := model.User{Id: id, Username: fmt.Sprint(id), AffCode: fmt.Sprint(id), Email: fmt.Sprintf("account%d@example.com", id), Status: common.UserStatusEnabled, Quota: 50}
		account.SetSetting(dto.UserSetting{Language: "en", QuotaWarningThreshold: 100})
		require.NoError(t, db.Create(&account).Error)
		queued, err := queueWalletNotice(ctx, id, time.Now(), id == 92, nil)
		require.NoError(t, err)
		require.True(t, queued)
		require.NoError(t, db.Model(&account).Update("quota", 120).Error)
	}
	var recipients []string
	require.NoError(t, deliverResourceAlerts(time.Now, func(_ context.Context, subject, recipient, body string) error {
		assert.Contains(t, subject, "A request was declined")
		recipients = append(recipients, recipient)
		return nil
	}))
	assert.Equal(t, []string{"account92@example.com"}, recipients)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 91).Update("quota", 90).Error)
	queued, err := queueWalletNotice(ctx, 91, time.Now(), false, nil)
	require.NoError(t, err)
	assert.True(t, queued, "recovery observed while delivering must rearm low balance notice")
}

func TestWalletDigestMembershipRollsBackWhenDeliveryCannotBeQueued(t *testing.T) {
	db := withErrorReportTestDB(t)
	ctx := context.Background()
	withErrorReportSetting(t, "ops@example.com")
	require.NoError(t, db.Create(&model.Option{Key: "mail_preferences", Value: `{"types":{"customer_wallet_rejection":{"enabled":true}}}`}).Error)
	end := time.Date(2026, 11, 15, 17, 30, 0, 0, errorReportLocation())
	require.NoError(t, db.Create(&model.ErrorEvent{
		UserId: 104, EventType: clienterrlog.EventAPIError, Status: 403,
		Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: end.Add(-time.Minute).Unix(),
	}).Error)
	require.NoError(t, db.Migrator().DropTable(&model.ResourceAlertDelivery{}))
	require.Error(t, queueWalletRejectionDigest(ctx, end.Add(time.Minute)))
	var count int64
	require.NoError(t, db.Model(&model.WalletDigestAccount{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&model.ResourceAlertEpisode{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.AutoMigrate(&model.ResourceAlertDelivery{}))
	require.NoError(t, queueWalletRejectionDigest(ctx, end.Add(2*time.Minute)))
	require.NoError(t, db.Model(&model.WalletDigestAccount{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}
