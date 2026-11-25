package model

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func arkCacheTestDB(t *testing.T, splitLogs bool) {
	t.Helper()
	previousDB, previousLogs := DB, LOG_DB
	open := func(name string, tables ...any) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(tables...))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
		return db
	}
	DB = open("main.db", &ArkPrefixCache{}, &ArkPrefixCacheHour{}, &User{}, &Token{}, &Log{}, &Channel{}, &QuotaData{})
	LOG_DB = DB
	if splitLogs {
		LOG_DB = open("logs.db", &Log{})
	}
	t.Cleanup(func() { DB, LOG_DB = previousDB, previousLogs })
}

func TestArkPrefixCacheHourlyEvidenceIsSettledOnce(t *testing.T) {
	for _, splitLogs := range []bool{false, true} {
		name := "same database"
		if splitLogs {
			name = "split database"
		}
		t.Run(name, func(t *testing.T) {
			arkCacheTestDB(t, splitLogs)
			ctx := t.Context()
			user := User{Username: "cache-pilot", Quota: 1000}
			require.NoError(t, DB.Create(&user).Error)
			token := Token{UserId: user.Id, Key: "cache-pilot-key", RemainQuota: 1000}
			require.NoError(t, DB.Create(&token).Error)
			channel := Channel{Name: "cache-pilot"}
			require.NoError(t, DB.Create(&channel).Error)
			const hour int64 = 36000
			cache := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "test", KeyDigest: strings.Repeat("a", 64),
				RequestDigest: strings.Repeat("b", 64), State: ArkCacheSending, UserID: user.Id, TokenID: token.Id,
				ChannelID: channel.Id, Model: "seed-lite", ExpiresAt: hour + 600, HeldQuota: 100, SentAt: hour + 10,
				PricePerMTokHr: "1", QuotaPerUnit: "500000", MaxStored: 200, MeterVersion: "reviewed-1",
				FundsDeadline: hour + 7200, NextActionAt: hour + 60}
			prior, err := ClaimArkPrefixCache(ctx, &cache, 4, 10, 100, hour+10)
			require.NoError(t, err)
			assert.Nil(t, prior)
			duplicate := cache
			duplicate.ID += "-duplicate"
			prior, err = ClaimArkPrefixCache(ctx, &duplicate, 4, 10, 100, hour+20)
			require.NoError(t, err)
			require.NotNil(t, prior)
			assert.Equal(t, cache.ID, prior.ID)
			cache.State, cache.CreatedAt, cache.NextHour = ArkCacheActive, hour+11, hour
			require.NoError(t, CompleteArkPrefixCache(ctx, cache))
			require.NoError(t, SettleArkPrefixCache(ctx, cache.ID, hour+3599))
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 900, user.Quota)
			assert.Zero(t, user.UsedQuota)
			evidence := ArkPrefixCacheHour{CacheID: cache.ID, HourStart: hour, Evidence: strings.Repeat("e", 64), Tokens: 20, OperatorID: user.Id}
			require.NoError(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601))
			require.NoError(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601))
			evidence.Tokens++
			require.ErrorIs(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601), ErrArkCacheConflict)
			require.NoError(t, SettleArkPrefixCache(ctx, cache.ID, hour+3602))
			require.NoError(t, SettleArkPrefixCache(ctx, cache.ID, hour+3702))
			require.NoError(t, DB.First(&user, user.Id).Error)
			require.NoError(t, DB.First(&token, token.Id).Error)
			assert.Equal(t, 990, user.Quota)
			assert.EqualValues(t, 10, user.UsedQuota)
			assert.Equal(t, 990, token.RemainQuota)
			assert.Zero(t, user.RequestCount)
			logs, err := ArkPrefixCacheLogsDue(ctx, hour+3700)
			require.NoError(t, err)
			require.Len(t, logs, 1)
			require.NoError(t, DeferArkPrefixCacheLog(ctx, logs[0].ID, hour+3700))
			dueBeforeRetry, err := ArkPrefixCacheLogsDue(ctx, hour+3760)
			require.NoError(t, err)
			require.Empty(t, dueBeforeRetry)
			require.ErrorIs(t, verifyArkCacheStatementSource(DB, user.Id, hour, hour+3600), ErrBillingStatementSourceIncomplete)
			require.NoError(t, DeliverArkPrefixCacheLog(ctx, logs[0].ID, hour+3820))
			if splitLogs {
				// The log database committed before the main database acknowledged it.
				require.NoError(t, DB.Model(&logs[0]).UpdateColumn("delivered_at", 0).Error)
			}
			require.NoError(t, DeliverArkPrefixCacheLog(ctx, logs[0].ID, hour+3821))
			require.NoError(t, verifyArkCacheStatementSource(DB, user.Id, hour, hour+3600))
			var ledger []Log
			require.NoError(t, LOG_DB.Find(&ledger).Error)
			require.Len(t, ledger, 1)
			assert.EqualValues(t, 10, ledger[0].Quota)
			_, summary, err := GetUserBillingStatement(user.Id, hour, hour+3599, token.Id, cache.Model)
			require.NoError(t, err)
			assert.EqualValues(t, 10, summary.NetQuota)
			assert.Zero(t, summary.Requests)
			assert.Zero(t, summary.TotalTokens)
			fact := upstreamBillingLogFact(ledger[0], ledger[0].Group)
			parsed := parseBillingReconciliationLog(fact)
			assert.Equal(t, ArkCacheBillingMode, parsed.billingMode)
			assert.Zero(t, billingStatementRequestCount(parsed))
			assert.Contains(t, ledger[0].Other, `"storage_tokens":20`)
			customerAmount, known := customerStatementOriginalQuota(fact, parsed)
			assert.True(t, known)
			assert.Equal(t, "10", customerAmount.String())
			_, upstreamReasons := upstreamOriginalQuota(fact, parsed)
			assert.NotEmpty(t, upstreamReasons)
			fact.RequestId = ""
			assert.Equal(t, ArkCacheBillingMode, parseBillingReconciliationLog(fact).billingMode)
		})
	}
}

func TestArkPrefixCacheUnknownDeadlineReleasesWithoutResolvingBusinessState(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	user := User{Username: "cache-unknown", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "cache-unknown-key", Status: common.TokenStatusEnabled, RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	cache := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "unknown", KeyDigest: strings.Repeat("a", 64),
		State: ArkCacheSending, UserID: user.Id, TokenID: token.Id, MeterVersion: "reviewed-1",
		SentAt: 100, ExpiresAt: 4200, HeldQuota: 20, FundsDeadline: 7800}
	_, err := ClaimArkPrefixCache(ctx, &cache, 5, 10, 20, 100)
	require.NoError(t, err)
	require.True(t, ArkCacheOutstanding(ctx))
	require.NoError(t, ReconcileStaleArkPrefixSends(ctx, 279))
	row, err := FindArkPrefixCache(ctx, user.Id, token.Id, cache.ID)
	require.NoError(t, err)
	require.Equal(t, ArkCacheSending, row.State)
	require.NoError(t, ReconcileStaleArkPrefixSends(ctx, 280))
	due, err := ArkPrefixCacheDue(ctx, 7800)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, cache.ID, due[0].ID)
	require.NoError(t, SettleArkPrefixCache(ctx, cache.ID, 7800))
	row, err = FindArkPrefixCache(ctx, user.Id, token.Id, cache.ID)
	require.NoError(t, err)
	assert.Equal(t, ArkCacheUnknown, row.State)
	assert.EqualValues(t, 7800, row.FundsClosedAt)
	assert.Zero(t, row.HeldQuota)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestArkSessionStorageEvidenceRequiresSettledInferenceAndFrozenMeter(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	const hour = int64(36000)
	row := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "fixture", UserID: 1, TokenID: 2, SessionRound: 1,
		State: ArkCacheActive, CreatedAt: hour + 1, ExpiresAt: hour + 60, InferenceSettledAt: hour + 2}
	require.NoError(t, DB.Create(&row).Error)
	evidence := ArkPrefixCacheHour{CacheID: row.ID, HourStart: hour, Evidence: strings.Repeat("a", 64), Tokens: 20}
	require.ErrorIs(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601), ErrArkCacheConflict)
	require.NoError(t, DB.Model(&row).Updates(map[string]any{"meter_version": "test-meter", "funds_deadline": hour + 7200,
		"max_stored": 20, "inference_settled_at": 0}).Error)
	require.ErrorIs(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601), ErrArkCacheConflict)
	require.NoError(t, DB.Model(&row).UpdateColumn("inference_settled_at", hour+2).Error)
	require.NoError(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601))
	require.NoError(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601))
	var hours []ArkPrefixCacheHour
	require.NoError(t, DB.Where("cache_id = ?", row.ID).Find(&hours).Error)
	require.Len(t, hours, 1)
	assert.Equal(t, 20, hours[0].Tokens)
}

func TestArkPrefixCacheEvidenceAboveFrozenCapPausesNewStorageHolds(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	user := User{Username: "cache-meter-pause", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "cache-meter-pause-key", RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	const hour int64 = 36000
	cache := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "pause", KeyDigest: strings.Repeat("a", 64),
		State: ArkCacheActive, UserID: user.Id, TokenID: token.Id, ChannelID: 1, MeterVersion: "reviewed-1",
		CreatedAt: hour + 1, ExpiresAt: hour + 60, FundsDeadline: hour + 7200, MaxStored: 256}
	require.NoError(t, DB.Create(&cache).Error)
	evidence := ArkPrefixCacheHour{CacheID: cache.ID, HourStart: hour, Evidence: strings.Repeat("e", 64), Tokens: 257}
	require.ErrorIs(t, RecordArkPrefixCacheHour(ctx, evidence, hour+3601), ErrArkCacheMeter)
	newCache := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "new", KeyDigest: strings.Repeat("b", 64),
		State: ArkCacheSending, UserID: user.Id, TokenID: token.Id, ChannelID: 1, MeterVersion: cache.MeterVersion,
		SentAt: hour + 3602, ExpiresAt: hour + 3700, HeldQuota: 20}
	_, err := ClaimArkPrefixCache(ctx, &newCache, 5, 10, 20, newCache.SentAt)
	require.ErrorIs(t, err, ErrArkCacheMeter)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 100, user.Quota)
	var count int64
	require.NoError(t, DB.Model(&ArkPrefixCacheHour{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestArkPrefixCacheMeterConflictStopsChargesAndReleasesAtDeadline(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	user := User{Username: "cache-small-hold", Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	token := Token{UserId: user.Id, Key: "cache-small-hold-key", RemainQuota: 100}
	require.NoError(t, DB.Create(&token).Error)
	const hour int64 = 36000
	row := ArkPrefixCache{ID: ArkPrefixCacheIDPrefix + "small-hold", KeyDigest: strings.Repeat("c", 64),
		State: ArkCacheSending, UserID: user.Id, TokenID: token.Id, ChannelID: 1, MeterVersion: "reviewed-1",
		ExpiresAt: hour + 60, FundsDeadline: hour + 7200, HeldQuota: 10, MaxStored: 300,
		PricePerMTokHr: "1", QuotaPerUnit: "500000", SentAt: hour + 1}
	_, err := ClaimArkPrefixCache(ctx, &row, 5, 10, 20, row.SentAt)
	require.NoError(t, err)
	row.State, row.CreatedAt, row.NextHour = ArkCacheActive, hour+1, hour
	require.NoError(t, CompleteArkPrefixCache(ctx, row))
	require.NoError(t, RecordArkPrefixCacheHour(ctx, ArkPrefixCacheHour{CacheID: row.ID, HourStart: hour,
		Evidence: strings.Repeat("a", 64), Tokens: 100}, hour+3600))
	require.ErrorIs(t, SettleArkPrefixCache(ctx, row.ID, hour+3600), ErrArkCacheMeter)
	due, err := ArkPrefixCacheDue(ctx, hour+3660)
	require.NoError(t, err)
	assert.Empty(t, due)
	require.ErrorIs(t, SettleArkPrefixCache(ctx, row.ID, hour+7200), ErrArkCacheMeter)
	require.NoError(t, SettleArkPrefixCache(ctx, row.ID, hour+7201))
	stored, err := FindArkPrefixCache(ctx, user.Id, token.Id, row.ID)
	require.NoError(t, err)
	assert.True(t, stored.MeterBlocked)
	assert.Equal(t, "meter_error", stored.FundsOutcome)
	assert.Zero(t, stored.HeldQuota)
	assert.Zero(t, stored.SettledQuota)
	assert.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
}
