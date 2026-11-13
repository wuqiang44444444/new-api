package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDailyReportKeepsPublishedUsageWhenLogsWereRemoved(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BatchJob{}))
	ctx := context.Background()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	view, before, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, view.Total.NetQuota)
	require.NoError(t, db.Where("user_id = ?", 7).Delete(&Log{}).Error)
	// A late insert can hide the deletion if publication only compares total rows.
	require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 2, Quota: 50}).Error)
	require.ErrorIs(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil), errDailyReportCoverageDecreased)
	view, after, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, view.Total.NetQuota)
	assert.Equal(t, before.References, after.References)
	assert.Equal(t, []string{date}, after.IncompleteDays)
}

func TestDailyReportStagesBothRefundDatesBeforeChangingPublishedVersions(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BatchJob{}))
	ctx := context.Background()
	refundDay := start + 2*billingReportDaySeconds
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	require.NoError(t, BuildBillingReportDay(ctx, refundDay, "billing-test", "runner", nil, nil))
	var oldUsage usageReportDay
	var oldBilling billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&oldUsage).Error)
	require.NoError(t, db.Where("start = ?", refundDay).Take(&oldBilling).Error)
	task := Task{TaskID: "cross-day", UserId: 7, FinishTime: start + 10, Platform: "video", Status: TaskStatusSuccess}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: task.ID, Event: "customer_refund", BeforeQuota: 100, AfterQuota: 70, CreatedAt: refundDay + 20, DeliveredAt: common.GetTimestamp() + 1}).Error)

	now := time.Now().In(UsageAnalyticsZone())
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	scopes, err := DailyReportScopes(ctx, refundDay, today, now.Unix(), false)
	require.NoError(t, err)
	require.Contains(t, scopes, DailyReportScope{Start: start, Kind: DailyReportUsage})
	require.Contains(t, scopes, DailyReportScope{Start: refundDay, Kind: DailyReportBilling})
	staged := StageDailyReport(ctx)
	require.NoError(t, BuildUsageReportDay(staged, date, "billing-test", "runner", nil))
	require.Error(t, PublishDailyReportStages(staged, scopes, "billing-test", "runner"))
	ready, err := GetDailyReportStage(ctx, DailyReportScope{Start: start, Kind: DailyReportUsage}, 0)
	require.NoError(t, err)
	require.True(t, ready)
	var usage usageReportDay
	require.NoError(t, db.Where("start = ?", start).Take(&usage).Error)
	assert.Equal(t, oldUsage.Generation, usage.Generation)

	require.NoError(t, BuildUsageReportDay(staged, time.Unix(refundDay, 0).In(UsageAnalyticsZone()).Format(time.DateOnly), "billing-test", "runner", nil))
	require.NoError(t, BuildBillingReportDay(staged, refundDay, "billing-test", "runner", nil, nil))
	require.NoError(t, PublishDailyReportStages(staged, scopes, "billing-test", "runner"))
	var bill billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&usage).Error)
	require.NoError(t, db.Where("start = ?", refundDay).Take(&bill).Error)
	assert.NotEqual(t, oldUsage.Generation, usage.Generation)
	assert.NotEqual(t, oldBilling.Generation, bill.Generation)
}

func TestDailyReportPublishesRefundDayWhenOlderUsageCoverageWasLost(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BatchJob{}))
	ctx := context.Background()
	refundDay := start + 2*billingReportDaySeconds
	zone := UsageAnalyticsZone()
	oldDate := time.Unix(start, 0).In(zone).Format(time.DateOnly)
	refundDate := time.Unix(refundDay, 0).In(zone).Format(time.DateOnly)
	original := Log{UserId: 7, ModelName: "video", Type: LogTypeConsume, CreatedAt: start + 10, Quota: 100}
	require.NoError(t, db.Create(&original).Error)
	require.NoError(t, BuildUsageReportDay(ctx, oldDate, "billing-test", "runner", nil))
	var old usageReportDay
	require.NoError(t, db.Where("start = ?", start).Take(&old).Error)
	require.NoError(t, db.Delete(&original).Error)
	require.NoError(t, BuildBillingReportDay(ctx, refundDay, "billing-test", "runner", nil, nil))
	task := Task{TaskID: "removed-source", UserId: 7, FinishTime: start + 10, Status: TaskStatusSuccess}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: task.ID, Event: "customer_refund", BeforeQuota: 100, AfterQuota: 70, CreatedAt: refundDay + 20, DeliveredAt: common.GetTimestamp() + 1}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 7, ModelName: "video", Type: LogTypeRefund, CreatedAt: refundDay + 20, Quota: 30},
		{UserId: 8, ModelName: "text", Type: LogTypeConsume, CreatedAt: refundDay + 21, Quota: 50},
	}).Error)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone).Unix()
	scopes, err := DailyReportScopes(ctx, refundDay, today, now.Unix(), false)
	require.NoError(t, err)
	require.Contains(t, scopes, DailyReportScope{Start: start, Kind: DailyReportUsage})
	staged := StageDailyReport(ctx)
	require.NoError(t, BuildUsageReportDay(staged, oldDate, "billing-test", "runner", nil))
	require.NoError(t, BuildUsageReportDay(staged, refundDate, "billing-test", "runner", nil))
	require.NoError(t, BuildBillingReportDay(staged, refundDay, "billing-test", "runner", nil, nil))
	require.NoError(t, PublishScheduledDailyReport(staged, scopes, "billing-test", "runner", refundDay))

	oldPeriod, err := ResolveUsageAnalyticsPeriod("day", oldDate, common.GetTimestamp())
	require.NoError(t, err)
	usage, meta, err := GetPublishedUsageCustomer(ctx, oldPeriod, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, usage.Total.NetQuota)
	assert.Equal(t, []UsageReportDayReference{{Start: start, Generation: old.Generation}}, meta.References)
	assert.Equal(t, []string{oldDate}, meta.IncompleteDays)
	billCtx, _, err := ResolveBillingReport(ctx, refundDay, refundDay+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	for userID, want := range map[int]int64{7: -30, 8: 50} {
		statement, err := GetBillingCustomerStatement(billCtx, userID, refundDay, refundDay+billingReportDaySeconds-1, "api_key", 0, "", "")
		require.NoError(t, err)
		assert.Equal(t, want, statement.Summary.NetQuota)
	}
	scopes, err = DailyReportScopes(ctx, refundDay, today, now.Unix(), false)
	require.NoError(t, err)
	assert.NotContains(t, scopes, DailyReportScope{Start: start, Kind: DailyReportUsage})
}

func TestDailyReportPruneKeepsPinnedUsageAndRejectsNewReadsBeforeDeletion(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BatchJob{}, &CustomerExportJob{}, &CustomerExportSlot{}))
	ctx := context.Background()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
	require.NoError(t, err)
	require.NoError(t, db.Create(&Log{UserId: 7, ModelName: "test", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	refs, err := FreezeUsageReportReferences(ctx, period, nil)
	require.NoError(t, err)
	require.NoError(t, db.Create(&CustomerExportSlot{ID: 1}).Error)
	job, created, err := CreateCustomerExportJob(7, 7, CustomerExportJobTypeUsageSummary, CustomerExportFilters{UsageReportDays: refs})
	require.NoError(t, err)
	require.True(t, created)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	pinned := WithUsageReportReferences(ctx, refs)
	now := common.GetTimestamp() + 9*86400
	pruned, err := PruneUsageReportRows(ctx, now)
	require.NoError(t, err)
	assert.False(t, pruned)
	view, _, err := GetPublishedUsageCustomer(pinned, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, view.Total.NetQuota)
	_, err = CancelCustomerExportJob(job.JobID, 7)
	require.NoError(t, err)
	pruned, err = PruneUsageReportRows(ctx, now)
	require.NoError(t, err)
	assert.False(t, pruned, "retirement first gives resolved readers time to finish")
	_, _, err = GetPublishedUsageCustomer(pinned, period, 7, "")
	require.Error(t, err)
	_, _, err = CreateCustomerExportJob(7, 7, CustomerExportJobTypeUsageSummary, CustomerExportFilters{UsageReportDays: refs})
	require.Error(t, err, "retired versions cannot be pinned by a new export")
	pruned, err = PruneUsageReportRows(ctx, now+61)
	require.NoError(t, err)
	assert.True(t, pruned)
}

func TestDailyReportSchedulerSkipsFailedYesterday(t *testing.T) {
	_, today, _ := setupBillingReportTest(t)
	ctx := context.Background()
	now := today + 5*3600
	for _, kind := range []string{DailyReportUsage, DailyReportBilling} {
		require.NoError(t, DeferDailyReportDate(ctx, today-86400, kind, errDailyReportCoverageDecreased))
	}
	day, _, due, _, err := NextDailyReportDay(ctx, today, now)
	require.NoError(t, err)
	require.True(t, due)
	assert.NotEqual(t, today-86400, day)
}

func TestDailyReportRejectsSameRowCountCorrectionOnPostgreSQL(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	if db.Dialector.Name() != "postgres" {
		t.Skip("requires PostgreSQL tuple revisions")
	}
	ctx := context.Background()
	row := Log{UserId: 7, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	staged := StageDailyReport(ctx)
	require.NoError(t, BuildBillingReportDay(staged, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Model(&row).Update("quota", 200).Error)
	scopes := []DailyReportScope{{Start: start, Kind: DailyReportBilling}}
	require.ErrorIs(t, PublishDailyReportStages(staged, scopes, "billing-test", "runner"), errDailyReportSourceChanged)
	pinned, _, err := ResolveBillingReport(ctx, start, start+86400-1, "")
	require.NoError(t, err)
	view, err := GetBillingCustomerStatement(pinned, 7, start, start+86400-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 100, view.Summary.NetQuota)
}

func TestConfirmedVersionFundGroupsPreserveNetAndRawLines(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&BillingStatementVersionLine{}))
	ctx := context.Background()
	parsed := parsedBillingReconciliationLog{taskID: "settled-video", customerModel: "video"}
	var rows []BillingStatementVersionLine
	for i, fact := range []billingReconciliationLog{
		{CreatedAt: start + 1, Type: LogTypeConsume, UserId: 7, TokenId: 11, Quota: 100},
		{CreatedAt: start + 2, Type: LogTypeRefund, UserId: 7, TokenId: 11, Quota: 30},
		{CreatedAt: start + 86401, Type: LogTypeRefund, UserId: 7, TokenId: 11, Quota: 5},
	} {
		policy := BillingStatementReadPolicy{ObserveFact: func(line BillingStatementVersionLine, _ *Log) error {
			line.VersionId, line.Sequence = 1, int64(i+1)
			rows = append(rows, line)
			return nil
		}}
		require.NoError(t, emitObservedFact(policy, fact, parsed, int64(i+1)))
	}
	rows = append(rows, BillingStatementVersionLine{VersionId: 1, Sequence: 4, CreatedAt: start + 1, Quota: 9, LogType: LogTypeConsume})
	require.NoError(t, db.Create(&rows).Error)
	groups, total, err := ListBillingStatementVersionFundGroups(ctx, 1, BillingStatementVersionLineFilter{}, 1, 2)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, groups, 2)
	assert.EqualValues(t, 9, groups[0].NetQuota, "legacy source without a proven relation remains separate")
	assert.EqualValues(t, -5, groups[1].NetQuota)
	groups, _, err = ListBillingStatementVersionFundGroups(ctx, 1, BillingStatementVersionLineFilter{}, 2, 2)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.EqualValues(t, 70, groups[0].NetQuota)
	lines, _, err := ListBillingStatementVersionLines(ctx, 1, BillingStatementVersionLineFilter{FundGroup: groups[0].Key}, 1, 20)
	require.NoError(t, err)
	require.Len(t, lines, 2)
	assert.EqualValues(t, 100, lines[0].Quota)
	assert.EqualValues(t, 30, lines[1].Quota)
}

func TestDailyReportRejectsFundingReattribution(t *testing.T) {
	for _, tc := range []struct{ name, field, value string }{
		{"customer model", "model_name", "text-b"},
		{"related task", "other", `{"task_id":"task-b","model_price":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, start, _ := setupBillingReportTest(t)
			ctx := context.Background()
			row := Log{UserId: 7, TokenId: 8, ChannelId: 9, ModelName: "text-a", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100, Other: `{"task_id":"task-a","model_price":1}`}
			require.NoError(t, db.Create(&row).Error)
			require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
			var before, after billingReportHead
			require.NoError(t, db.Where("start = ?", start).Take(&before).Error)
			require.NoError(t, db.Model(&row).Update(tc.field, tc.value).Error)
			assert.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportCoverageDecreased)
			require.NoError(t, db.Where("start = ?", start).Take(&after).Error)
			assert.Equal(t, before.Generation, after.Generation)
		})
	}
}

func TestDailyReportRollbackRejectsFundingReattribution(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	row := Log{UserId: 7, TokenId: 8, ChannelId: 9, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100, Other: `{"task_id":"task-a","model_price":1}`}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	var prior, current billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&prior).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Where("start = ?", start).Take(&current).Error)
	require.NotEqual(t, prior.Generation, current.Generation)
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	// Simulate a generation that was published before the identity guard.
	previousLine := db.Model(&billingReportLine{}).Where("generation = ? AND source_id = ?", prior.Generation, row.Id)
	require.NoError(t, previousLine.Update("fund_group", billingStatementFundIdentity(row, parsedBillingReconciliationLog{customerModel: "text", taskID: "task-b"})).Error)
	require.ErrorIs(t, RestoreBillingReportGeneration(ctx, date, current.Generation, prior.Generation), errDailyReportCoverageDecreased)
	var head billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	assert.Equal(t, current.Generation, head.Generation)
	require.NoError(t, previousLine.Update("fund_group", billingStatementFundIdentity(row, parsedBillingReconciliationLog{customerModel: "text", taskID: "task-a"})).Error)
	require.NoError(t, RestoreBillingReportGeneration(ctx, date, current.Generation, prior.Generation))
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	assert.Equal(t, prior.Generation, head.Generation)
}

func TestDailyReportPruneCannotUnpublishRestoredHead(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	require.NoError(t, db.AutoMigrate(&CustomerExportJob{}))
	ctx := context.Background()
	row := Log{UserId: 7, TokenId: 8, ModelName: "text", Type: LogTypeConsume, CreatedAt: start + 1, Quota: 100}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	var prior, current billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&prior).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Where("start = ?", start).Take(&current).Error)
	require.NotEqual(t, prior.Generation, current.Generation)
	now := common.GetTimestamp()
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("id = ?", prior.Generation).Update("retired_at", now-9*86400).Error)
	var restoreErr error
	fired := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("review:prune-race", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "billing_report_generations" {
			return
		}
		fired = true
		restoreErr = RestoreBillingReportGeneration(ctx, time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly), current.Generation, prior.Generation)
	}))
	_, err := PruneBillingReportGenerations(ctx, now)
	require.NoError(t, err)
	require.True(t, fired)
	require.NoError(t, restoreErr)
	var head billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
	require.Equal(t, prior.Generation, head.Generation)
	var generation billingReportGeneration
	require.NoError(t, db.Where("id = ?", head.Generation).Take(&generation).Error)
	assert.True(t, generation.Published)
	_, _, err = ResolveBillingReport(ctx, start, start+86399, head.Generation)
	assert.NoError(t, err)
}
