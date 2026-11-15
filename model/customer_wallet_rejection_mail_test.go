package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestWalletRejectionSummaryAcrossDatabases(t *testing.T) {
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
			}
			old, oldLog := DB, LOG_DB
			oldType := common.MainDatabaseType()
			DB, LOG_DB = db, db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			t.Cleanup(func() { DB, LOG_DB = old, oldLog; common.SetMainDatabaseType(oldType) })
			require.NoError(t, db.AutoMigrate(&ErrorEvent{}, &ResourceAlertEpisode{}, &ResourceAlertDelivery{}, &WalletDigestAccount{}, &Option{}))
			require.NoError(t, db.Create(&[]ErrorEvent{
				{UserId: 7, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 149},
				{UserId: 7, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 151},
				{UserId: 8, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 150},
				{UserId: 9, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 152},
				{UserId: 9, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 153},
				{UserId: 9, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, CreatedAt: 200},
				{UserId: 10, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.APIKeyQuotaInsufficient, CreatedAt: 155},
			}).Error)
			ctx := context.Background()
			require.NoError(t, db.Create(&Option{Key: mailPreferencesOption, Value: `{"types":{"customer_wallet_rejection":{"enabled":true}}}`}).Error)
			// Only a committed membership entry, rather than an older log, counts as prior inclusion.
			require.NoError(t, db.Create(&WalletDigestAccount{DayStart: 100, UserID: 7}).Error)
			counts, err := ReadWalletRejectionCounts(ctx, 150, 200)
			require.NoError(t, err)
			prepared, err := SelectWalletDigestAccounts(db, 100, counts)
			require.NoError(t, err)
			assert.Equal(t, []WalletRejectionCount{{9, 2}, {8, 1}}, prepared)
			key := "test-wallet-digest"
			episode := ResourceAlertEpisode{OpenKey: &key, Resource: CustomerWalletRejectionMail, OpenedAt: 200}
			require.NoError(t, PublishWalletRejectionDigest(ctx, 100, time.Unix(200, 0), 0, []string{"ops@example.com"}, counts, prepared, episode))
			require.NoError(t, PublishWalletRejectionDigest(ctx, 100, time.Unix(210, 0), 0, []string{"ops@example.com"}, counts, prepared, episode))
			var members []WalletDigestAccount
			require.NoError(t, db.Order("user_id").Find(&members).Error)
			assert.Equal(t, []WalletDigestAccount{{100, 7}, {100, 8}, {100, 9}}, members)
			var deliveries int64
			require.NoError(t, db.Model(&ResourceAlertDelivery{}).Count(&deliveries).Error)
			assert.EqualValues(t, 1, deliveries)
		})
	}
}
