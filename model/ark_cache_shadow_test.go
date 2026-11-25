package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkCacheShadowSeparatesPeakFromSumAndMarksUnknown(t *testing.T) {
	arkCacheTestDB(t, false)
	ctx := t.Context()
	start := common.GetTimestamp()/3600*3600 - 7200
	base := ArkPrefixCache{UserID: 31, TokenID: 50, ChannelID: 79, Model: "seed", Endpoint: "test-target",
		Credential: "test-scope-digest"}
	for _, sample := range []struct {
		name, state                  string
		from, to                     int64
		input, cached, output, round int
	}{
		{"prefix-a", ArkCacheDeleted, 10, 110, 100, 0, 0, 0},
		{"prefix-b", ArkCacheDeleted, 110, 3600, 200, 0, 0, 0},
		{"session-one", ArkCacheAbsent, 3500, 3900, 50, 0, 8, 1},
		{"session-two", ArkCacheActive, 3620, 3900, 75, 50, 5, 2},
		{"unknown", ArkCacheUnknown, 3660, 3750, 0, 0, 0, 2},
	} {
		row := base
		row.ID, row.KeyDigest, row.State = sample.name, ArkSessionCacheKeyDigest(sample.name), sample.state
		row.CreatedAt, row.SentAt, row.ExpiresAt = start+sample.from, start+sample.from, start+sample.to
		row.RequestedUntil = row.ExpiresAt
		row.InputTokens, row.ReadTokens, row.OutputTokens, row.SessionRound = sample.input, sample.cached, sample.output, sample.round
		if sample.state == ArkCacheDeleted {
			row.DeletedAt = row.ExpiresAt
		}
		if sample.state == ArkCacheAbsent {
			row.DeleteRequestedAt = start + 3610
		}
		if sample.state != ArkCacheUnknown {
			row.ProviderID = "resp_" + sample.name
		}
		require.NoError(t, DB.Create(&row).Error)
	}

	hours, err := ArkCacheShadowHours(ctx, 79, 31, 50, "seed", start, start+7200)
	require.NoError(t, err)
	require.Len(t, hours, 2)
	assert.Equal(t, ArkCacheShadowHour{HourStart: start, PrefixPeakCandidate: 200, PrefixSumCandidate: 300,
		SessionPeakCandidate: 58, SessionSumCandidate: 58}, hours[0])
	assert.Equal(t, ArkCacheShadowHour{HourStart: start + 3600, SessionPeakCandidate: 88, SessionSumCandidate: 88,
		UnverifiedWrites: 2}, hours[1])
}
