package model

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyReportPublishesStagedAndLegacyIssueCounts(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}}
	ctx := t.Context()
	billed := Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeConsume, CreatedAt: start, Quota: 100,
		Other: `{"admin_info":{"statement_original_quota":140}}`}
	require.NoError(t, db.Create(&billed).Error)

	stageCtx := StageDailyReport(ctx)
	require.NoError(t, BuildBillingReportDay(stageCtx, start, "billing-test", "runner", nil, nil))
	var first dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, DailyReportBilling).Take(&first).Error)
	require.NotNil(t, first.IssueRows)
	assert.Zero(t, *first.IssueRows)
	assert.Equal(t, first.Generation, first.IssueRowsGeneration)

	countQueries := 0
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:issue_count_outside_publish", func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if strings.Contains(sql, "billing_report_lines") && strings.Contains(sql, "previous_original_missing") &&
			strings.Contains(strings.ToUpper(sql), "COUNT(") {
			countQueries++
			if _, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter); inTransaction {
				tx.AddError(errors.New("diagnostic issue count ran inside a transaction"))
			}
		}
	}))
	require.NoError(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"))
	assert.Zero(t, countQueries, "a complete zero never triggers a recount")

	// Preserve a previous correction even when the new source has no replay
	// or price issue. The count must use the written row after it is enriched.
	require.NoError(t, db.Model(&billingReportLine{}).Where("generation = ? AND source_id = ?", first.Generation, billed.Id).
		Update("previous_original", "120").Error)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1,
		Quota: 40, Other: "{}"}).Error)
	for _, oldMetadata := range []map[string]any{
		{"issue_rows": nil},
		{"issue_rows": 0, "issue_rows_generation": first.Generation},
	} {
		require.NoError(t, BuildBillingReportDay(stageCtx, start, "billing-test", "runner", nil, nil))
		var rebuilt dailyReportStage
		require.NoError(t, db.Where("start = ? AND kind = ?", start, DailyReportBilling).Take(&rebuilt).Error)
		require.NotNil(t, rebuilt.IssueRows)
		assert.EqualValues(t, 2, *rebuilt.IssueRows)
		assert.Equal(t, rebuilt.Generation, rebuilt.IssueRowsGeneration)

		// Older workers can leave a missing count or the preceding generation's count.
		require.NoError(t, db.Model(&rebuilt).Updates(oldMetadata).Error)
		require.NoError(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"))
	}
	assert.Equal(t, 2, countQueries)
	days, err := BillingReportDiagnosticDays(ctx, start, start+billingReportDaySeconds)
	require.NoError(t, err)
	require.Len(t, days, 1)
	assert.EqualValues(t, 2, days[0].IssueRows)
	lines, err := BillingReportDiagnosticLines(ctx, start, 0)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.Equal(t, "120", lines[0].PreviousOriginal)
	assert.Empty(t, lines[0].ReplayIssue)
	assert.NotEmpty(t, lines[1].ReplayIssue)
	pinned, _, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 140, statement.Summary.NetQuota)
}

func TestDailyReportCoverageMatchesIndependentLogBoundaries(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	end := start + billingReportDaySeconds
	logs := []Log{
		{CreatedAt: start - 1, Type: LogTypeConsume},
		{CreatedAt: start, Type: LogTypeConsume},
		{CreatedAt: start + 3599, Type: LogTypeError},
		{CreatedAt: start + 3600, Type: LogTypeRefund},
		{CreatedAt: start + 3600, Type: LogTypeSystem},
		{CreatedAt: end - 1, Type: LogTypeConsume},
		{CreatedAt: end, Type: LogTypeConsume},
	}
	require.NoError(t, db.Create(&logs).Error)
	currentUpper, billingUpper, usageUpper := int64(logs[3].Id), int64(logs[5].Id), int64(logs[2].Id)
	require.NoError(t, db.Create(&billingReportGeneration{ID: "prior-billing", Start: start,
		UpperLogID: billingUpper, Rows: 3, HistoricalSource: dailyReportSourceDetails}).Error)
	require.NoError(t, db.Create(&usageReportDay{Start: start, Generation: "prior-usage",
		UpperLogID: usageUpper, SourceRows: 2}).Error)
	stages := []dailyReportStage{
		{Start: start, Kind: DailyReportBilling, Previous: "prior-billing", UpperLogID: int64(logs[6].Id),
			SourceRows: 2, OriginalReplayVersion: CustomerOriginalReplayVersion},
		{Start: start, Kind: DailyReportUsage, Previous: "prior-usage", UpperLogID: int64(logs[6].Id),
			SourceRows: 3, UsageMeterVersion: usageReportMeterVersion},
	}
	for i := range stages {
		scope := stages[i]
		want, err := readDailyReportCoverage(ctx, dailyReportLogQuery(ctx, start, end, currentUpper, dailyReportLogTypes(scope.Kind)))
		require.NoError(t, err)
		require.Equal(t, scope.SourceRows, want.Rows)
		stages[i].SourceRevision = want.Revision
	}
	bounded := context.WithValue(ctx, dailyReportLogBoundaryKey{}, currentUpper)
	checks, err := readDailyReportStageLogChecks(bounded, stages)
	require.NoError(t, err)
	for i, stage := range stages {
		assert.Equal(t, dailyReportCoverage{Rows: stage.SourceRows, Revision: stage.SourceRevision}, checks[i].current)
	}
	assert.EqualValues(t, 3, checks[0].previousRows, "the old billing boundary may exceed the current bound")
	assert.EqualValues(t, 2, checks[1].previousRows, "usage has its own previous bound and log types")

	// Without a cycle-wide bound, each stage must retain its individual upper ID.
	stages[0].UpperLogID, stages[1].UpperLogID = currentUpper, billingUpper
	for i, stage := range stages {
		want, err := readDailyReportCoverage(ctx, dailyReportLogQuery(ctx, start, end, stage.UpperLogID, dailyReportLogTypes(stage.Kind)))
		require.NoError(t, err)
		stages[i].SourceRows, stages[i].SourceRevision = want.Rows, want.Revision
	}
	checks, err = readDailyReportStageLogChecks(ctx, stages)
	require.NoError(t, err)
	assert.EqualValues(t, 2, checks[0].current.Rows)
	assert.EqualValues(t, 4, checks[1].current.Rows)
	for i, stage := range stages {
		assert.Equal(t, stage.SourceRevision, checks[i].current.Revision)
	}

	empty, err := readDailyReportDateCoverage(ctx, start, DailyReportBilling, int64(logs[0].Id))
	require.NoError(t, err)
	assert.Equal(t, dailyReportLogScanCoverage(ctx, 0, 0), empty)
}

func TestDailyReportJointPublishRejectsOldCoverageLossDespiteReplacement(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := t.Context()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}, {Start: start, Kind: DailyReportUsage}}
	original := Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}
	oldError := Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeError, CreatedAt: start + 2, RequestId: "original-error"}
	require.NoError(t, db.Create(&original).Error)
	require.NoError(t, db.Create(&oldError).Error)

	build := func() {
		staged, err := WithDailyReportLogBoundary(StageDailyReport(ctx))
		require.NoError(t, err)
		require.NoError(t, BuildBillingReportDay(staged, start, "billing-test", "runner", nil, nil))
		require.NoError(t, BuildUsageReportDay(staged, date, "billing-test", "runner", nil))
	}
	build()
	require.NoError(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"))
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeConsume,
		CreatedAt: start + 3600, Quota: 50}).Error)
	build()
	require.NoError(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"))
	var billing billingReportHead
	var usage usageReportDay
	require.NoError(t, db.Where("start = ?", start).Take(&billing).Error)
	require.NoError(t, db.Where("start = ?", start).Take(&usage).Error)

	require.NoError(t, db.Delete(&oldError).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 11, ModelName: "text", Type: LogTypeError,
		CreatedAt: start + billingReportDaySeconds - 1, RequestId: "later-error"}).Error)
	build()
	require.ErrorIs(t, PublishDailyReportStages(ctx, scopes, "billing-test", "runner"), errDailyReportCoverageDecreased)
	var stayedBilling billingReportHead
	var stayedUsage usageReportDay
	require.NoError(t, db.Where("start = ?", start).Take(&stayedBilling).Error)
	require.NoError(t, db.Where("start = ?", start).Take(&stayedUsage).Error)
	assert.Equal(t, billing.Generation, stayedBilling.Generation)
	assert.Equal(t, usage.Generation, stayedUsage.Generation)

	pinned, _, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	statement, err := GetBillingCustomerStatement(pinned, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 150, statement.Summary.NetQuota)
}
