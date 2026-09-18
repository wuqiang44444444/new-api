package model

import (
	"context"
	"io"
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
	var rows []ErrorEvent
	names, err := WalkErrorEventsForReport(context.Background(), 3600, 7200, func(e *ErrorEvent) error { rows = append(rows, *e); return nil })
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.Equal(t, "fixture", names[7])
	require.NoError(t, db.Create(&ErrorReportSchedule{Scope: ErrorReportScheduleScopeGlobal, Enabled: true, Periods: `[{"start":3600,"end":0}]`}).Error)
	report := ErrorReport{ReportID: "report", WindowStart: 3600, WindowEnd: 7200, Status: ErrorReportStatusBuilding}
	_, err = InsertErrorReportBuilding(&report)
	require.NoError(t, err)
	partNo := 0
	require.NoError(t, PublishErrorReportParts(context.Background(), report.ReportID, func() (*ErrorReportPart, error) {
		partNo++
		if partNo > 2 {
			return nil, io.EOF
		}
		return &ErrorReportPart{ReportID: report.ReportID, PartNo: partNo, BodyHTML: "fixture"}, nil
	}, []string{"a@example.com", "b@example.com"}, 2, "{}", ""))
	// Concurrent workers must not claim the same recipient/part or skip ahead.
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
