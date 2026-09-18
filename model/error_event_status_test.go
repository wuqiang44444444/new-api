package model

import (
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the persisted/query contract, including old rows without body snapshots.
func TestErrorEventStatusFilterMatchesDisplayedStatus(t *testing.T) {
	setupErrorEventTestDB(t)
	verifyErrorEventStatusFilter(t)
}

func TestErrorEventStatusFilterServerContract(t *testing.T) {
	dialect := os.Getenv("TEST_BILLING_STATEMENT_DIALECT")
	if dialect == "" {
		t.Skip("requires isolated server opt-in")
	}
	db := setupBillingStatementServerDB(t, dialect)
	oldDB, oldLog := DB, LOG_DB
	oldMain, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, db
	kind := common.DatabaseTypeMySQL
	if dialect == "postgres" {
		kind = common.DatabaseTypePostgreSQL
	}
	common.SetDatabaseTypes(kind, kind)
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog; common.SetDatabaseTypes(oldMain, oldLogType) })
	require.NoError(t, MigrateErrorEvents())
	verifyErrorEventStatusFilter(t)
}

func verifyErrorEventStatusFilter(t *testing.T) {
	t.Helper()
	fixtures := []struct {
		id, eventType, upstream string
		status                  int
	}{
		{"manual-500", "channel_test", "500", 200},
		{"auto-500", "channel_test", "500", 0},
		{"manual-400", "channel_test", "400", 200},
		{"auto-no-response", "channel_test", "", 0},
		{"manual-no-response", "channel_test", "", 200},
		{"stream-200", "stream_error", "500", 200},
		{"api-500", "api_error", "400", 500},
		{"task-no-response", "task_failure", "500", 0},
		{"upstream-200-decode-failure", "channel_test", "200", 200},
		{"non-http-provider-code", "channel_test", "666", 0},
	}
	for i, fixture := range fixtures {
		e := clienterrlog.Event{At: time.Unix(1000+int64(i), 0), Module: "relay", RequestID: fixture.id,
			EventType: fixture.eventType, Status: fixture.status}
		if fixture.upstream != "" {
			e.Detail = map[string]string{"upstream_status": fixture.upstream}
		}
		require.NoError(t, persistErrorEvent(e))
	}
	for _, tc := range []struct {
		status int
		ids    []string
	}{
		{200, []string{"upstream-200-decode-failure", "stream-200"}},
		{500, []string{"api-500", "auto-500", "manual-500"}},
		{400, []string{"manual-400"}},
		{0, []string{"non-http-provider-code", "task-no-response", "manual-no-response", "auto-no-response"}},
		{503, []string{}},
	} {
		events, total, err := GetErrorEvents(ErrorEventFilter{Status: &tc.status}, 0, 20)
		require.NoError(t, err)
		ids := make([]string, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.RequestId)
		}
		assert.Equal(t, tc.ids, ids, "status=%d", tc.status)
		assert.EqualValues(t, len(tc.ids), total)
	}
	status := 500
	events, total, err := GetErrorEvents(ErrorEventFilter{Status: &status, EventType: "channel_test"}, 1, 1)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, "manual-500", events[0].RequestId)
	assert.Equal(t, 200, events[0].Status, "keep the actual management HTTP status unchanged")
	events, total, err = GetErrorEvents(ErrorEventFilter{Status: &status, RequestId: "manual-400"}, 0, 20)
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Zero(t, total)
}
