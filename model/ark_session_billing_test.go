package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestArkSessionWalletReceiptAndLogSurviveRetry(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			switch dialect {
			case common.DatabaseTypeSQLite:
				arkCacheTestDB(t, true)
			case common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL:
				env := "TEST_ARK_SESSION_MYSQL_DSN"
				if dialect == common.DatabaseTypePostgreSQL {
					env = "TEST_ARK_SESSION_POSTGRES_DSN"
				}
				dsn := os.Getenv(env)
				if dsn == "" {
					t.Skip(env + " needs an isolated test database")
				}
				var driver gorm.Dialector = mysql.Open(dsn)
				if dialect == common.DatabaseTypePostgreSQL {
					driver = postgres.Open(dsn)
				}
				db, err := gorm.Open(driver, &gorm.Config{})
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
				previousDB, previousLog := DB, LOG_DB
				DB, LOG_DB = db, db
				t.Cleanup(func() { DB, LOG_DB = previousDB, previousLog })
				require.NoError(t, db.AutoMigrate(&ArkPrefixCache{}, &ArkPrefixCacheHour{}, &User{}, &Token{}, &Log{}, &Channel{}, &QuotaData{}))
			}

			priorMain, priorLog := common.MainDatabaseType(), common.LogDatabaseType()
			priorLogEnabled := common.LogConsumeEnabled
			priorExportEnabled := common.DataExportEnabled
			common.SetDatabaseTypes(dialect, dialect)
			common.LogConsumeEnabled = true
			common.DataExportEnabled = true
			t.Cleanup(func() {
				common.SetDatabaseTypes(priorMain, priorLog)
				common.LogConsumeEnabled = priorLogEnabled
				common.DataExportEnabled = priorExportEnabled
			})

			ctx := t.Context()
			now := common.GetTimestamp() - 10
			user := User{Username: "ark-session-test", Quota: 1000}
			require.NoError(t, DB.Create(&user).Error)
			token := Token{UserId: user.Id, Key: "ark-session-test-key", RemainQuota: 1000}
			require.NoError(t, DB.Create(&token).Error)
			channel := Channel{Name: "ark-session-test"}
			require.NoError(t, DB.Create(&channel).Error)
			round := ArkPrefixCache{
				ID: ArkSessionCacheIDPrefix + "root", UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id,
				KeyDigest: ArkSessionCacheKeyDigest("root"), RequestDigest: ArkSessionCacheKeyDigest("body"),
				Model: "seed", Credential: ArkSessionCacheKeyDigest("verified-test-scope"), SessionRound: 1, State: ArkCacheSending,
				SentAt: now, ExpiresAt: now + 360, RequestedUntil: now + 360, InferenceNextAt: now + 180, InferenceHeldQuota: 100,
			}
			prior, err := ClaimArkPrefixCache(ctx, &round, 8, 20, 0, now)
			require.NoError(t, err)
			require.Nil(t, prior)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 900, user.Quota)
			round.State, round.ProviderID, round.CreatedAt = ArkCacheInferencePending, "resp_fixture", now
			round.InferenceQuota, round.InferenceLog = 37, `{"cache_tokens":40,"billing_source":"wallet"}`
			round.InputTokens, round.ReadTokens, round.OutputTokens = 60, 40, 6
			require.NoError(t, CompleteArkPrefixCache(ctx, round))
			stored, err := FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			assert.Equal(t, ArkCacheInferencePending, stored.State)
			assert.Equal(t, 100, stored.InferenceHeldQuota)

			require.NoError(t, SettleArkSessionInference(ctx, round.ID, common.GetTimestamp()))
			require.NoError(t, SettleArkSessionInference(ctx, round.ID, common.GetTimestamp()))
			require.NoError(t, DB.First(&user, user.Id).Error)
			require.NoError(t, DB.First(&token, token.Id).Error)
			assert.Equal(t, 963, user.Quota)
			assert.Equal(t, int64(37), int64(user.UsedQuota))
			assert.EqualValues(t, 1, user.RequestCount)
			assert.Equal(t, 963, token.RemainQuota)
			assert.EqualValues(t, 37, token.UsedQuota)
			stored, err = FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			assert.Equal(t, ArkCacheActive, stored.State)
			assert.Equal(t, 0, stored.HeldQuota)
			if dialect == common.DatabaseTypeSQLite {
				require.NoError(t, LOG_DB.Migrator().DropTable(&Log{}))
				require.Error(t, DeliverArkSessionLog(ctx, round.ID))
				require.NoError(t, LOG_DB.AutoMigrate(&Log{}))
				require.NoError(t, DB.Migrator().DropTable(&QuotaData{}))
				require.Error(t, DeliverArkSessionLog(ctx, round.ID))
				var committed int64
				require.NoError(t, LOG_DB.Model(&Log{}).Where("request_id = ?", "ark-session:root").Count(&committed).Error)
				assert.EqualValues(t, 1, committed)
				require.NoError(t, DB.AutoMigrate(&QuotaData{}))
			}
			common.LogConsumeEnabled = false
			require.NoError(t, DeliverArkSessionLog(ctx, round.ID))
			require.NoError(t, DeliverArkSessionLog(ctx, round.ID))
			var logs []Log
			require.NoError(t, LOG_DB.Where("request_id = ?", "ark-session:root").Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, 37, logs[0].Quota)
			assert.Equal(t, 60, logs[0].PromptTokens)
			var dashboardRows []QuotaData
			require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&dashboardRows).Error)
			require.Len(t, dashboardRows, 1)
			assert.Equal(t, 37, dashboardRows[0].Quota)

			child := ArkPrefixCache{
				ID: ArkSessionCacheIDPrefix + "child", UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id,
				Model: round.Model, ParentID: round.ID, Credential: round.Credential, SessionRound: 2,
				KeyDigest: ArkSessionCacheKeyDigest("child"), RequestDigest: ArkSessionCacheKeyDigest("payload"),
				State: ArkCacheSending, SentAt: now, ExpiresAt: now + 360, RequestedUntil: now + 360,
				InferenceNextAt: now + 180, InferenceHeldQuota: 50,
			}
			prior, err = ClaimArkPrefixCache(ctx, &child, 8, 20, 0, now)
			require.NoError(t, err)
			require.Nil(t, prior)
			prior, err = ClaimArkPrefixCache(ctx, &child, 8, 20, 0, now)
			require.NoError(t, err)
			require.NotNil(t, prior)
			assert.Equal(t, child.ID, prior.ID)
			branch := child
			branch.ID, branch.KeyDigest = ArkSessionCacheIDPrefix+"branch", ArkSessionCacheKeyDigest("another-key")
			_, err = ClaimArkPrefixCache(ctx, &branch, 8, 20, 0, now)
			assert.ErrorIs(t, err, ErrArkCacheBusy)
			child.State, child.RejectedStatus = ArkCacheRejected, 429
			require.NoError(t, CompleteArkPrefixCache(ctx, child))
			require.NoError(t, SettleArkSessionInference(ctx, child.ID, common.GetTimestamp()))
			stored, err = FindArkPrefixCache(ctx, user.Id, token.Id, child.ID)
			require.NoError(t, err)
			require.Equal(t, ArkCacheRejected, stored.State)
			original, err := ClaimArkPrefixCache(ctx, &child, 8, 20, 0, now)
			require.NoError(t, err)
			require.Equal(t, child.ID, original.ID)
			require.Equal(t, 429, original.RejectedStatus)
			replacement, err := ClaimArkPrefixCache(ctx, &branch, 8, 20, 0, now)
			require.NoError(t, err)
			require.Nil(t, replacement)
			require.NoError(t, CompleteArkPrefixCache(ctx, child))
			parent, err := FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			require.Equal(t, branch.ID, parent.SessionNextID)
			require.NoError(t, SettleArkSessionInference(ctx, branch.ID, now+180))
			require.NoError(t, SettleArkSessionInference(ctx, branch.ID, now+180))
			stored, err = FindArkPrefixCache(ctx, user.Id, token.Id, branch.ID)
			require.NoError(t, err)
			assert.Equal(t, ArkCacheUnknown, stored.State)
			assert.Equal(t, 0, stored.InferenceHeldQuota)
			another := branch
			another.ID, another.KeyDigest = ArkSessionCacheIDPrefix+"third-key", ArkSessionCacheKeyDigest("third-key")
			_, err = ClaimArkPrefixCache(ctx, &another, 8, 20, 0, now+181)
			assert.ErrorIs(t, err, ErrArkCacheBusy)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 963, user.Quota)

			stalePrefix := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "stale", UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id,
				KeyDigest: ArkPrefixCacheKeyDigest("stale"), State: ArkCacheSending, SentAt: now,
				ExpiresAt: now + 60, FundsDeadline: now + 360, HeldQuota: 25}
			_, err = ClaimArkPrefixCache(ctx, &stalePrefix, 8, 20, 25, now)
			require.NoError(t, err)
			require.NoError(t, ReconcileStaleArkPrefixSends(ctx, now+180))
			require.NoError(t, SettleArkPrefixCache(ctx, stalePrefix.ID, now+360))
			stored, err = FindArkPrefixCache(ctx, user.Id, token.Id, stalePrefix.ID)
			require.NoError(t, err)
			assert.Equal(t, ArkCacheUnknown, stored.State)
			assert.Zero(t, stored.HeldQuota)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 963, user.Quota)
		})
	}
}

func TestArkCacheUnknownDeletionKeepsBillingCutoffUnknown(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	now := common.GetTimestamp()
	row := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "deletion", ProviderID: "resp_fixture",
		State: ArkCacheActive, CreatedAt: now, ExpiresAt: now + 360}
	require.NoError(t, DB.Create(&row).Error)
	require.NoError(t, BeginArkPrefixCacheDelete(ctx, row, now))
	taken, err := ClaimArkCacheDeletionCheck(ctx, row.ID, now+74)
	require.NoError(t, err)
	assert.False(t, taken)
	taken, err = ClaimArkCacheDeletionCheck(ctx, row.ID, now+75)
	require.NoError(t, err)
	assert.True(t, taken)
	require.NoError(t, ObserveArkCacheAbsent(ctx, row.ID, now+76))
	row, err = FindArkCacheForCleanup(ctx, row.ID)
	require.NoError(t, err)
	assert.Equal(t, ArkCacheAbsent, row.State)
	assert.Equal(t, now+76, row.AbsentAt)
	assert.Zero(t, row.DeletedAt)
	assert.Equal(t, now+360, arkCacheEnd(row))
}
