package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestArkCacheChannelHourlyLimitAcrossKeys(t *testing.T) {
	for _, backend := range []struct {
		database common.DatabaseType
		env      string
		open     func(string) gorm.Dialector
	}{
		{common.DatabaseTypeSQLite, "", sqlite.Open},
		{common.DatabaseTypeMySQL, "TEST_ARK_CACHE_MYSQL_DSN", mysql.Open},
		{common.DatabaseTypePostgreSQL, "TEST_ARK_CACHE_POSTGRES_DSN", postgres.Open},
	} {
		t.Run(string(backend.database), func(t *testing.T) {
			var dsn string
			if backend.env == "" {
				dsn = t.TempDir() + "/ark-admission.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate"
			} else {
				dsn = os.Getenv(backend.env)
				if dsn == "" {
					t.Skipf("%s is not configured", backend.env)
				}
			}
			db, err := gorm.Open(backend.open(dsn), &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			tables := []any{&ArkPrefixCache{}, &Token{}, &Channel{}, &User{}}
			for _, table := range tables {
				require.False(t, db.Migrator().HasTable(table), "use an empty test database")
			}
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(tables...)) })
			require.NoError(t, db.AutoMigrate(tables...))
			previousDB, previousType, previousRedis := DB, common.MainDatabaseType(), common.RedisEnabled
			DB = db
			common.SetMainDatabaseType(backend.database)
			common.RedisEnabled = false
			t.Cleanup(func() {
				DB, common.RedisEnabled = previousDB, previousRedis
				common.SetMainDatabaseType(previousType)
			})
			verifyArkCacheChannelHourlyLimit(t, backend.database)
		})
	}
}

func verifyArkCacheChannelHourlyLimit(t *testing.T, database common.DatabaseType) {
	t.Helper()
	channel := Channel{Name: "shared-cache-channel"}
	require.NoError(t, DB.Create(&channel).Error)
	const hour = int64(36000)
	const now = hour + 15
	const hourlyLimit = 2
	const initialQuota, heldQuota = 100, 10

	// An attempt that reached the provider continues to count after rejection.
	seed := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "rejected", ChannelID: channel.Id,
		State: ArkCacheRejected, SessionRound: 1, SentAt: hour + 1}
	require.NoError(t, DB.Create(&seed).Error)
	claims := make([]ArkPrefixCache, 0, 2)
	for _, name := range []string{"first", "second"} {
		user := User{Username: name, AffCode: name, Quota: initialQuota}
		require.NoError(t, DB.Create(&user).Error)
		key := Token{UserId: user.Id, Key: name, RemainQuota: initialQuota}
		require.NoError(t, DB.Create(&key).Error)
		claims = append(claims, ArkPrefixCache{ID: ArkSessionCacheIDPrefix + name, KeyDigest: ArkSessionCacheKeyDigest(name),
			RequestDigest: ArkSessionCacheKeyDigest("request:" + name), UserID: user.Id, TokenID: key.Id,
			ChannelID: channel.Id, State: ArkCacheSending,
			SessionRound: 1, SentAt: now, ExpiresAt: now + 120, InferenceHeldQuota: heldQuota})
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if database != common.DatabaseTypeSQLite {
		// On server databases both Keys can pass the initial quota reads before
		// either commits. Pause their independent wallet updates at that point.
		walletUpdates := make(chan struct{}, len(claims))
		resume := make(chan struct{})
		require.NoError(t, DB.Callback().Update().After("gorm:update").Register("test:ark_wallet_barrier", func(tx *gorm.DB) {
			if tx.Statement.Table == "users" && tx.Error == nil {
				walletUpdates <- struct{}{}
				select {
				case <-resume:
				case <-ctx.Done():
					_ = tx.AddError(ctx.Err())
				}
			}
		}))
		defer func() { require.NoError(t, DB.Callback().Update().Remove("test:ark_wallet_barrier")) }()
		go func() {
			defer close(resume)
			for range claims {
				select {
				case <-walletUpdates:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	start := make(chan struct{})
	type result struct {
		claim ArkPrefixCache
		err   error
	}
	results := make(chan result, len(claims))
	for _, claim := range claims {
		go func() {
			<-start
			_, err := ClaimArkPrefixCache(ctx, &claim, 8, hourlyLimit, 0, arkCacheTestClock(now))
			results <- result{claim, err}
		}()
	}
	close(start)
	outcomes := []result{<-results, <-results}
	wins := 0
	for _, outcome := range outcomes {
		balance := initialQuota
		row, err := FindArkPrefixCache(ctx, outcome.claim.UserID, outcome.claim.TokenID, outcome.claim.ID)
		if outcome.err == nil {
			wins++
			balance -= heldQuota
			require.NoError(t, err)
			assert.Equal(t, ArkCacheSending, row.State)
		} else {
			assert.ErrorIs(t, outcome.err, ErrArkCacheLimit)
			assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
		var user User
		require.NoError(t, DB.First(&user, outcome.claim.UserID).Error)
		assert.Equal(t, balance, user.Quota)
		var key Token
		require.NoError(t, DB.First(&key, outcome.claim.TokenID).Error)
		assert.Equal(t, balance, key.RemainQuota)
	}
	require.Equal(t, 1, wins)
	var attempts int64
	require.NoError(t, DB.Model(&ArkPrefixCache{}).Where("channel_id = ?", channel.Id).Count(&attempts).Error)
	assert.EqualValues(t, hourlyLimit, attempts)

	for _, outcome := range outcomes {
		claim := outcome.claim
		if outcome.err == nil {
			prior, err := ClaimArkPrefixCache(ctx, &claim, 8, hourlyLimit, 0, arkCacheTestClock(now))
			require.NoError(t, err)
			require.NotNil(t, prior)
			assert.Equal(t, claim.ID, prior.ID)
			continue
		}
		// The rejected Key can retry with the same idempotency key once the
		// next natural hour starts. It has no prior claim or remaining hold.
		claim.SentAt, claim.ExpiresAt = hour+3600, hour+3720
		prior, err := ClaimArkPrefixCache(ctx, &claim, 8, hourlyLimit, 0, arkCacheTestClock(claim.SentAt))
		require.NoError(t, err)
		assert.Nil(t, prior)
	}
}

func TestArkCacheAdmissionUsesHourOfSendPermission(t *testing.T) {
	arkCacheTestDB(t, false)
	channel := Channel{Name: "cache-hour"}
	user := User{Username: "cache-hour", Quota: 100}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "cache-hour", RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	const nextHour int64 = 36000
	seed := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "next-hour", ChannelID: channel.Id, SentAt: nextHour}
	require.NoError(t, DB.Create(&seed).Error)
	claim := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "waiting", UserID: user.Id, TokenID: token.Id,
		ChannelID: channel.Id, State: ArkCacheSending, KeyDigest: ArkSessionCacheKeyDigest("waiting"),
		SentAt: nextHour - 1, ExpiresAt: nextHour + 60, InferenceHeldQuota: 10, InferenceNextAt: nextHour + 179}
	call := 0
	clock := func() int64 {
		call++
		if call == 1 {
			return nextHour - 1
		}
		return nextHour
	}
	_, err := ClaimArkPrefixCache(t.Context(), &claim, 8, 1, 0, clock)
	require.ErrorIs(t, err, ErrArkCacheLimit)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	call = 0
	_, err = ClaimArkPrefixCache(t.Context(), &claim, 8, 2, 0, clock)
	require.NoError(t, err)
	stored, err := FindArkPrefixCache(t.Context(), user.Id, token.Id, claim.ID)
	require.NoError(t, err)
	assert.Equal(t, nextHour, stored.SentAt)
	assert.Equal(t, nextHour+180, stored.InferenceNextAt)
}
