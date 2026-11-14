package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailPreferencesMigratesLegacyEffectiveValuesOnlyOnce(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	t.Setenv("PERSONAL_USAGE_REPORT_MAIL_ENABLED", "false")
	require.NoError(t, db.Create(&Option{Key: "notification_setting.send_email_on_account_created", Value: "true"}).Error)
	require.NoError(t, MigrateMailPreferences())
	ctx := context.Background()
	prefs, err := LoadMailPreferences(ctx)
	require.NoError(t, err)
	assert.True(t, prefs.Enabled(MailWelcome))
	assert.False(t, prefs.Enabled(MailTokenCreated))
	assert.False(t, prefs.Enabled(MailPersonalUsage))
	assert.True(t, prefs.Enabled(MailDailyErrors))
	t.Setenv("PERSONAL_USAGE_REPORT_MAIL_ENABLED", "true")
	_, err = UpdateMailPreferences(ctx, map[MailType]bool{MailWelcome: false}, nil)
	require.NoError(t, err)
	require.NoError(t, MigrateMailPreferences())
	prefs, err = LoadMailPreferences(ctx)
	require.NoError(t, err)
	assert.False(t, prefs.Enabled(MailWelcome))
	assert.False(t, prefs.Enabled(MailPersonalUsage))
	assert.True(t, prefs.LegacyLifecycleChannels[MailWelcome])
	require.ErrorIs(t, UpdateOption("notification_setting.send_email_on_account_created", "true"), ErrInvalidMailPreferences)
	_, err = UpdateMailPreferences(ctx, map[MailType]bool{"unknown": true}, nil)
	require.ErrorIs(t, err, ErrInvalidMailPreferences)
}

func TestMailPreferencesStopsPendingDeliveryAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "sqlite" {
				db = setupErrorEventTestDB(t)
			} else {
				if os.Getenv("TEST_MAIL_PREFERENCES_DIALECT") != dialect {
					t.Skip("isolated server not configured")
				}
				db = setupBillingStatementServerDB(t, dialect)
				oldDB, oldType := DB, common.MainDatabaseType()
				DB = db
				common.SetMainDatabaseType(common.DatabaseType(dialect))
				t.Cleanup(func() { DB = oldDB; common.SetMainDatabaseType(oldType) })
			}
			require.NoError(t, db.AutoMigrate(&Option{}, &ChannelAlertEpisode{}, &ChannelAlertDelivery{}, &ResourceAlertEpisode{}, &ResourceAlertDelivery{}, &ErrorReportDelivery{}))
			require.NoError(t, MigrateMailPreferences())
			channel := ChannelAlertEpisode{Kind: "upstream_balance"}
			require.NoError(t, db.Create(&channel).Error)
			row := ChannelAlertDelivery{EpisodeID: channel.ID, Recipient: "ops@example.com", Status: MailDeliveryRetryWait}
			require.NoError(t, db.Create(&row).Error)
			resource := ResourceAlertEpisode{Resource: "cpu"}
			require.NoError(t, db.Create(&resource).Error)
			resourceRow := ResourceAlertDelivery{EpisodeID: resource.ID, Recipient: "ops@example.com", Status: MailDeliveryPending}
			require.NoError(t, db.Create(&resourceRow).Error)
			daily := ErrorReportDelivery{Status: MailDeliveryPending}
			require.NoError(t, db.Create(&daily).Error)
			ctx := context.Background()
			finance := "finance@example.com"
			_, err := UpdateMailPreferences(ctx, map[MailType]bool{MailChannelBalance: false, MailCPU: false, MailDailyErrors: false}, &finance)
			require.NoError(t, err)
			require.NoError(t, db.First(&row, row.ID).Error)
			require.NoError(t, db.First(&resourceRow, resourceRow.ID).Error)
			require.NoError(t, db.First(&daily, daily.ID).Error)
			assert.Equal(t, MailStopPreferenceDisabled, row.StopReason)
			assert.Equal(t, MailStopPreferenceDisabled, resourceRow.StopReason)
			assert.Equal(t, MailStopPreferenceDisabled, daily.StopReason)
			addresses, err := FinanceMailRecipients(db)
			require.NoError(t, err)
			assert.Equal(t, []string{finance}, addresses)
		})
	}
}
