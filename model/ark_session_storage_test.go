package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkSessionLocalStorageReservesAndSettlesClosedHours(t *testing.T) {
	previousAllowed := arkSessionLocalBillingAllowed
	arkSessionLocalBillingAllowed = func() bool { return true }
	t.Cleanup(func() { arkSessionLocalBillingAllowed = previousAllowed })
	for _, test := range []struct {
		name          string
		unknownDelete bool
		charged       int
		released      int
		hours         int
		outcome       string
	}{
		{name: "two closed hours", charged: 68, hours: 2, outcome: "metered"},
		{name: "unconfirmed delete stops the affected hour", unknownDelete: true, charged: 34, released: 34, hours: 1, outcome: "deadline"},
	} {
		t.Run(test.name, func(t *testing.T) {
			arkCacheTestDB(t, true)
			previousQuotaPerUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 500000
			t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
			ctx := t.Context()
			user := User{Username: "storage-pilot", Quota: 1000}
			require.NoError(t, DB.Create(&user).Error)
			token := Token{UserId: user.Id, Key: "storage-test-key", RemainQuota: 1000}
			require.NoError(t, DB.Create(&token).Error)
			channel := Channel{Name: "storage-pilot"}
			require.NoError(t, DB.Create(&channel).Error)
			const hour int64 = 1756656000
			round := ArkPrefixCache{
				ID: ArkSessionCacheIDPrefix + "local", KeyDigest: ArkSessionCacheKeyDigest("local"),
				UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id, Model: "seed",
				State: ArkCacheSending, SessionRound: 1, SentAt: hour + 3540, ExpiresAt: hour + 3660,
				InferenceHeldQuota: 100, MeterVersion: ArkSessionLocalMeterVersion,
			}
			require.NoError(t, round.ReserveArkSessionLocalStorage("1", 100))
			_, err := ClaimArkPrefixCache(ctx, &round, 8, 10, 100, round.SentAt)
			require.NoError(t, err)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 800, user.Quota)

			round.State, round.ProviderID, round.CreatedAt = ArkCacheInferencePending, "resp_example", round.SentAt
			round.InputTokens, round.ReadTokens, round.OutputTokens = 100, 40, 8
			round.InferenceQuota, round.InferenceLog = 37, `{"billing_source":"wallet"}`
			require.NoError(t, CompleteArkPrefixCache(ctx, round))
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, round.FundsDeadline+1))
			pending, err := FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			assert.Equal(t, 100, pending.HeldQuota)
			assert.Zero(t, pending.FundsClosedAt)
			require.NoError(t, SettleArkSessionInference(ctx, round.ID, common.GetTimestamp()))
			saved, err := FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			assert.Equal(t, 68, saved.HeldQuota)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 895, user.Quota)
			require.ErrorIs(t, verifyArkCacheStatementSource(DB, user.Id, hour, hour+3600), ErrBillingStatementSourceIncomplete)

			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, hour+3599))
			var evidence []ArkPrefixCacheHour
			require.NoError(t, DB.Find(&evidence).Error)
			require.Empty(t, evidence)
			require.ErrorIs(t, RecordArkPrefixCacheHour(ctx, ArkPrefixCacheHour{
				CacheID: round.ID, HourStart: hour, Tokens: 68,
			}, hour+3600), ErrArkCacheConflict)
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, hour+3600))
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, hour+3600))
			require.NoError(t, DB.Find(&evidence).Error)
			require.Len(t, evidence, 1)
			assert.Equal(t, 68, evidence[0].Tokens)
			assert.Equal(t, 34, evidence[0].Quota)
			require.NoError(t, DeliverArkPrefixCacheLog(ctx, evidence[0].ID, hour+3600))
			require.NoError(t, DeliverArkSessionLog(ctx, round.ID))
			require.NoError(t, verifyArkCacheStatementSource(DB, user.Id, hour, hour+3600))
			if test.unknownDelete {
				require.NoError(t, BeginArkPrefixCacheDelete(ctx, saved, hour+3610))
				require.NoError(t, ObserveArkCacheAbsent(ctx, round.ID, hour+3620))
			}
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, hour+7200))
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, round.FundsDeadline))
			require.NoError(t, SettleArkPrefixCache(ctx, round.ID, round.FundsDeadline+60))
			saved, err = FindArkPrefixCache(ctx, user.Id, token.Id, round.ID)
			require.NoError(t, err)
			assert.Equal(t, test.charged, saved.SettledQuota)
			assert.Equal(t, test.released, saved.ReleasedQuota)
			assert.Equal(t, test.outcome, saved.FundsOutcome)
			assert.Zero(t, saved.HeldQuota)
			require.NoError(t, DB.Order("hour_start").Find(&evidence).Error)
			require.Len(t, evidence, test.hours)
			for i, event := range evidence {
				assert.Equal(t, hour+3600*int64(i), event.HourStart)
				assert.Equal(t, 68, event.Tokens)
				assert.Zero(t, event.OperatorID)
			}
			require.NoError(t, DB.First(&user, user.Id).Error)
			require.NoError(t, DB.First(&token, token.Id).Error)
			assert.Equal(t, 1000-37-test.charged, user.Quota)
			assert.Equal(t, user.Quota, token.RemainQuota)
			assert.EqualValues(t, 1, user.RequestCount)
		})
	}
}

func TestArkSessionLocalStorageDeleteUsesBillableIntersection(t *testing.T) {
	previousAllowed := arkSessionLocalBillingAllowed
	arkSessionLocalBillingAllowed = func() bool { return true }
	t.Cleanup(func() { arkSessionLocalBillingAllowed = previousAllowed })
	const hour int64 = 1756656000
	for _, test := range []struct {
		name               string
		state              string
		created, expiry    int64
		requested, deleted int64
		wantHours          int64
	}{
		{name: "same second", state: ArkCacheDeleted, created: hour + 10, expiry: hour + 90, deleted: hour + 10},
		{name: "exact next hour", state: ArkCacheDeleted, created: hour + 3590, expiry: hour + 3630, deleted: hour + 3600, wantHours: 1},
		{name: "absent after expiry", state: ArkCacheAbsent, created: hour + 10, expiry: hour + 60, requested: hour + 90, wantHours: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			arkCacheTestDB(t, false)
			user := User{Username: "storage-delete", Quota: 90}
			require.NoError(t, DB.Create(&user).Error)
			round := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "delete", UserID: user.Id, SessionRound: 1,
				MeterVersion: ArkSessionLocalMeterVersion, PricePerMTokHr: "1", QuotaPerUnit: "500000",
				State: test.state, CreatedAt: test.created, ExpiresAt: test.expiry, DeleteRequestedAt: test.requested, DeletedAt: test.deleted,
				InputTokens: 20, InferenceSettledAt: test.created, HeldQuota: 10,
				NextHour: hour, NextActionAt: hour + 3600, FundsDeadline: hour + 10800}
			require.NoError(t, DB.Create(&round).Error)
			require.NoError(t, SettleArkPrefixCache(t.Context(), round.ID, hour+7200))
			var count int64
			require.NoError(t, DB.Model(&ArkPrefixCacheHour{}).Count(&count).Error)
			assert.Equal(t, test.wantHours, count)
			require.NoError(t, DB.First(&user, user.Id).Error)
			assert.Equal(t, 100-int(test.wantHours)*10, user.Quota)
		})
	}
}

func TestArkSessionLocalStorageIgnoresRestoredTestRowsOutsideIsolation(t *testing.T) {
	previousAllowed := arkSessionLocalBillingAllowed
	arkSessionLocalBillingAllowed = func() bool { return false }
	t.Cleanup(func() { arkSessionLocalBillingAllowed = previousAllowed })
	arkCacheTestDB(t, true)
	ctx := t.Context()
	const hour int64 = 1756656000
	row := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "restored", MeterVersion: ArkSessionLocalMeterVersion,
		State: ArkCacheInferencePending, SessionRound: 1, UserID: 1, CreatedAt: hour + 10, ExpiresAt: hour + 60,
		NextHour: hour, NextActionAt: hour + 3600, InferenceNextAt: hour + 3600, HeldQuota: 10,
		InferenceHeldQuota: 20, InferenceLogDueAt: hour + 3600}
	require.NoError(t, DB.Create(&row).Error)
	event := ArkPrefixCacheHour{CacheID: row.ID, HourStart: hour, SettledAt: hour + 3600}
	require.NoError(t, DB.Create(&event).Error)
	now := hour + 7200
	storage, err := ArkPrefixCacheDue(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, storage)
	inference, err := ArkSessionInferenceDue(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, inference)
	logs, err := ArkPrefixCacheLogsDue(ctx, now)
	require.NoError(t, err)
	assert.Empty(t, logs)
	require.NoError(t, SettleArkPrefixCache(ctx, row.ID, now))
	require.NoError(t, SettleArkSessionInference(ctx, row.ID, now))
	require.NoError(t, DeliverArkPrefixCacheLog(ctx, event.ID, now))
	require.NoError(t, DeliverArkSessionLog(ctx, row.ID))
	require.NoError(t, verifyArkCacheStatementSource(DB, row.UserID, hour, now))
	assert.False(t, ArkCacheOutstanding(ctx))
	require.NoError(t, DB.First(&row, "id = ?", row.ID).Error)
	assert.Equal(t, 10, row.HeldQuota)
	assert.Equal(t, 20, row.InferenceHeldQuota)
	assert.Zero(t, row.FundsClosedAt)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.Zero(t, count)
}
