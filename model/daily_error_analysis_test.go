package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyReportCountsCustomerSignalsWhenProbesFillRanking(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := LOG_DB
	LOG_DB = db
	t.Cleanup(func() { LOG_DB = previous })
	require.NoError(t, db.AutoMigrate(&ErrorEvent{}))

	// Distinct probe channels can fill the ranking, but sparse customer
	// failures and stream disconnects must still be counted correctly.
	rows := make([]ErrorEvent, 0, 263)
	for channel := 1; channel <= 130; channel++ {
		probe := ErrorEvent{CreatedAt: 100, EventType: clienterrlog.EventChannelTest, ChannelId: channel, Status: 200}
		rows = append(rows, probe, probe)
	}
	rows = append(rows,
		ErrorEvent{CreatedAt: 101, EventType: clienterrlog.EventAPIError, Status: 503},
		ErrorEvent{CreatedAt: 102, EventType: clienterrlog.EventStreamError, Status: 200, Reason: "client_disconnected"},
		ErrorEvent{CreatedAt: 103, EventType: clienterrlog.EventStreamError, Status: 200, Reason: "stream_timeout"},
	)
	require.NoError(t, db.CreateInBatches(&rows, 50).Error)
	slice, err := ReadDailyReportSlice(context.Background(), 100, 400)
	require.NoError(t, err)
	assert.EqualValues(t, 263, slice.Total)
	assert.Equal(t, map[string]int64{
		clienterrlog.EventChannelTest: 260,
		DailyErrorAPI5xx:              1,
		DailyErrorStreamDisconnected:  1,
		clienterrlog.EventStreamError: 1,
	}, slice.Signals)
	assert.True(t, slice.GroupsCapped)
	require.NotEmpty(t, slice.Groups)
	assert.Equal(t, DailyErrorAPI5xx, slice.Groups[0].Signal())
}
