package model

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Uses the same explicit isolated-server opt-in as billing verification.
func TestErrorReportServerContract(t *testing.T) {
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
	initCol()
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog; common.SetDatabaseTypes(oldMain, oldLogType); initCol() })
	require.NoError(t, db.AutoMigrate(&ErrorReport{}, &ErrorReportPart{}, &ErrorReportSchedule{}, &ErrorReportDelivery{}, &ErrorEvent{}, &Channel{}))
	require.NoError(t, db.Create(&Channel{Id: 7, Name: "fixture"}).Error)
	require.NoError(t, db.Create(&[]ErrorEvent{{CreatedAt: 3600, RequestId: "duplicate", ChannelId: 7}, {CreatedAt: 3600, RequestId: "duplicate", ChannelId: 7}, {CreatedAt: 7200, RequestId: "outside"}}).Error)
	ctx := context.Background()
	slice, err := ReadDailyReportSlice(ctx, 3600, 7200)
	require.NoError(t, err)
	assert.EqualValues(t, 2, slice.Total)
	assert.Equal(t, []DailyReportChannelCount{{ChannelID: 7, Count: 2}}, slice.Channels)
	require.NoError(t, db.Create(&ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal, DailyWindowStart: 3600}).Error)
	report, err := LoadDailyReport(ctx, 3600, "report")
	require.NoError(t, err)
	require.NoError(t, SaveDailyReportProgress(ctx, report, report.WindowEnd, "{}", 2))
	require.NoError(t, PublishDailyReport(ctx, &ErrorReportPart{
		ReportID: report.ReportID, PartNo: 1, BodyHTML: "fixture",
	}, []string{"a@example.com", "b@example.com"}, 2, "{}", ""))
	// Concurrent workers must not claim the same recipient/part or send twice.
	type result struct {
		rows []*ErrorReportDelivery
		err  error
	}
	results := make(chan result, 4)
	var wg sync.WaitGroup
	for _, owner := range []string{"one", "two", "three", "four"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			claimed, err := ClaimErrorReportDeliveries(7200, owner, 7300, 1)
			results <- result{claimed, err}
		}(owner)
	}
	wg.Wait()
	close(results)
	claimed := map[string]bool{}
	for result := range results {
		require.NoError(t, result.err)
		for _, row := range result.rows {
			assert.False(t, claimed[row.Recipient])
			claimed[row.Recipient] = true
			assert.Equal(t, 1, row.PartNo)
		}
	}
	assert.Len(t, claimed, 2)
}
