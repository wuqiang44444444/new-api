package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Period resolution tests: Asia/Shanghai day boundaries, Monday-based natural
// weeks, cross-month weeks, and strict parameter rejection.
func TestResolveUsageAnalyticsPeriodDay(t *testing.T) {
	// 2026-09-16 is a Wednesday; 2026-09-20 is a Sunday in Asia/Shanghai.
	now := int64(1789488000 + 3600) // 2026-09-16 01:00 +08:00
	period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", now)
	require.NoError(t, err)
	assert.Equal(t, "2026-09-16", period.Date)
	assert.Equal(t, int64(1), int64(len(period.Days)))
	assert.EqualValues(t, 1789488000, period.StartTimestamp) // 2026-09-16 00:00 +08:00
	assert.EqualValues(t, 1789488000+86400, period.EndTimestamp)
	assert.False(t, period.Days[0].Future)
}

func TestResolveUsageAnalyticsPeriodWeekCrossMonth(t *testing.T) {
	// 2026-11-01 is a Sunday; its natural week starts Monday 2026-10-26 and
	// crosses the October/November month boundary.
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-11-01", 0)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-26", period.Days[0].Date)
	assert.Equal(t, "2026-11-01", period.Days[6].Date)
	require.Len(t, period.Days, 7)
	assert.EqualValues(t, period.StartTimestamp, period.Days[0].Start)
	assert.EqualValues(t, period.EndTimestamp, period.Days[0].Start+7*86400)
}

func TestResolveUsageAnalyticsPeriodFutureDay(t *testing.T) {
	// now == 2026-09-16 12:00 +08:00 (1757995200): the same day is not future,
	// the next day is.
	now := int64(1789488000 + 12*3600)
	period, err := ResolveUsageAnalyticsPeriod("week", "2026-09-16", now)
	require.NoError(t, err)
	require.Len(t, period.Days, 7)
	assert.False(t, period.Days[2].Future) // Wednesday 2026-09-16, index 2
	assert.True(t, period.Days[3].Future)  // Thursday 2026-09-17
	assert.False(t, period.Days[0].Future)
}

func TestResolveUsageAnalyticsPeriodRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		period string
		date   string
	}{{"month", "2026-09-16"}, {"", "2026-09-16"}, {"day", "2026/09/16"}, {"day", ""}, {"day", "2026-13-40"}} {
		_, err := ResolveUsageAnalyticsPeriod(tc.period, tc.date, 0)
		assert.Error(t, err, "period=%q date=%q must be rejected", tc.period, tc.date)
	}
}
