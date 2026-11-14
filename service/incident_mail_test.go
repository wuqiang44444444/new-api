package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncidentSQLReadFailurePreservesFactsAndRetries(t *testing.T) {
	dialect := os.Getenv("DAILY_REPORT_TEST_DIALECT")
	if dialect == "" {
		dialect = "sqlite"
	}
	db := channelAlertDialectDB(t, dialect)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Channel{}, &model.ChannelAlertEpisode{},
		&model.ChannelAlertDelivery{}, &model.ResourceAlertEpisode{}, &model.ResourceAlertDelivery{}))
	ctx := context.Background()
	now := time.Now()
	channel := model.Channel{Name: "incident-sql-fixture"}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Migrator().RenameTable("options", "unavailable_options"))
	t.Cleanup(func() {
		if db.Migrator().HasTable("unavailable_options") {
			assert.NoError(t, db.Migrator().RenameTable("unavailable_options", "options"))
		}
	})
	recipient := []string{"ops@example.com"}
	key := "sql-error-fixture"
	opened, err := model.OpenChannelAlert(ctx, model.ChannelAlertEpisode{
		OpenKey: &key, ScopeKey: key, ChannelID: channel.Id, Revision: model.ChannelAlertRevision(&channel),
		Kind: "upstream_balance", FirstAt: now.UnixMilli(), LastAt: now.UnixMilli(),
	}, recipient)
	require.NoError(t, err)
	require.True(t, opened)
	opened, err = model.OpenResourceAlert(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: "cpu", OpenedAt: now.Unix(), Subject: "cpu incident",
	}, recipient)
	require.NoError(t, err)
	require.True(t, opened)
	require.NoError(t, db.Migrator().RenameTable("unavailable_options", "options"))
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: recipient[0]}).Error)
	for range 2 {
		require.NoError(t, model.RetryIncidentMail(ctx, true))
		require.NoError(t, model.RetryIncidentMail(ctx, false))
	}
	channelDelivery, err := model.ClaimChannelAlertDelivery(ctx, "channel", now.UnixMilli(), now.Add(time.Minute).UnixMilli())
	require.NoError(t, err)
	require.NotNil(t, channelDelivery)
	assert.Equal(t, recipient[0], channelDelivery.Recipient)
	resourceDelivery, err := model.ClaimResourceAlertDelivery(ctx, "resource", now.Unix(), now.Add(time.Minute).Unix())
	require.NoError(t, err)
	require.NotNil(t, resourceDelivery)
	assert.Equal(t, recipient[0], resourceDelivery.Recipient)
}

func TestIncidentMailRetriesPreparationAfterConfigurationRecovers(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	ctx := context.Background()
	now := time.Now()
	channel := model.Channel{Name: "incident-mail-fixture"}
	require.NoError(t, db.Create(&channel).Error)
	key := "incident-mail-fixture"
	episode := model.ChannelAlertEpisode{
		OpenKey: &key, ScopeKey: key, ChannelID: channel.Id, Revision: model.ChannelAlertRevision(&channel),
		Kind: "upstream_balance", FirstAt: now.UnixMilli(), LastAt: now.UnixMilli(),
	}
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "invalid address"}).Error)
	require.NoError(t, confirmChannelAlert(ctx, episode, now))
	var saved model.ChannelAlertEpisode
	require.NoError(t, db.Take(&saved).Error)
	require.Error(t, model.RetryIncidentMail(ctx, true))
	// Even a recovered incident remains eligible for preparation after the
	// recipient lookup recovers.
	require.NoError(t, model.CloseChannelAlert(ctx, saved.ID, now.Add(time.Second).UnixMilli(), model.ChannelAlertClosedRecovered))

	withErrorReportSetting(t, "ops@example.com")
	require.NoError(t, db.Create(&model.Option{Key: "mail_preferences", Value: "invalid"}).Error)
	health := &model.EvidenceCleanupHealth{Alerts: []string{"cleanup_stalled"}}
	require.NoError(t, recordEvidenceCleanupHealth(ctx, now.Unix(), health, false))
	var before []model.ResourceAlertEpisode
	require.NoError(t, db.Find(&before).Error)
	require.Len(t, before, 1)
	require.Error(t, model.RetryIncidentMail(ctx, false))
	require.NoError(t, db.Delete(&model.Option{Key: "mail_preferences"}).Error)

	var subjects []string
	send := func(_ context.Context, subject, recipient, _ string) error {
		assert.Equal(t, "ops@example.com", recipient)
		subjects = append(subjects, subject)
		return nil
	}
	for range 2 {
		require.NoError(t, model.RetryIncidentMail(ctx, true))
		_, err := deliverChannelAlertMail("incident-mail", send, time.Now)
		require.NoError(t, err)
		require.NoError(t, deliverResourceAlerts(time.Now, send))
	}
	require.Len(t, subjects, 2)
	assert.Contains(t, subjects[0], "Upstream channel balance alert")
	assert.Contains(t, subjects[1], "Evidence cleanup alert")
}

func TestWalletFinanceRecipientFailureRollsBackAdjustment(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.WalletOperation{}))
	t.Setenv("FINANCE_MAIL_RECIPIENTS", "invalid address")
	user := model.User{Username: "wallet-rollback", Email: "account@example.com", Quota: 100}
	require.NoError(t, db.Create(&user).Error)
	_, err := model.AdjustUserQuotaWithMail(user.Id, common.RoleRootUser, "subtract", 100, 42, QueueWalletOperationMail)
	require.Error(t, err)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	var count int64
	require.NoError(t, db.Model(&model.WalletOperation{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&model.ResourceAlertDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
}
