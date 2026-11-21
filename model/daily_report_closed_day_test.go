package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportDefersCrossDayRefundUntilBothDaysCanPublish(t *testing.T) {
	db, finishDay, _ := setupBillingReportTest(t)
	ctx := t.Context()
	refundDay := finishDay + 2*billingReportDaySeconds
	task := Task{TaskID: "cross-day-close", UserId: 7, Status: TaskStatusSuccess, FinishTime: finishDay + 10}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: task.ID, Event: "customer_refund",
		BeforeQuota: 100, AfterQuota: 70, CreatedAt: refundDay + 20, DeliveredAt: refundDay + 30}).Error)
	// A second refund on the earlier task-day links to the open day even
	// when the primary date is the previously closed refund occurrence.
	require.NoError(t, db.Create(&TaskBillingDelivery{TaskRowID: task.ID, Event: "settlement",
		BeforeQuota: 70, AfterQuota: 60, CreatedAt: refundDay - billingReportDaySeconds + 20, DeliveredAt: refundDay - billingReportDaySeconds + 30}).Error)
	_, err := DailyReportScopes(ctx, refundDay-billingReportDaySeconds, refundDay, refundDay+4*3600, false)
	require.ErrorIs(t, err, ErrDailyReportDeferred)

	for _, window := range []struct{ today, now int64 }{
		{refundDay, refundDay + 60},
		{refundDay + billingReportDaySeconds, refundDay + billingReportDaySeconds + dailyReportCloseDelay - 1},
	} {
		_, err := DailyReportScopes(ctx, finishDay, window.today, window.now, false)
		require.ErrorIs(t, err, ErrDailyReportDeferred)
	}
	today := refundDay + billingReportDaySeconds
	require.NoError(t, db.Create(&billingReportRebuild{Start: refundDay, RequestedAt: 1}).Error)
	scopes, err := DailyReportScopes(ctx, finishDay, today, today+dailyReportCloseDelay, false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []DailyReportScope{
		{Start: finishDay, Kind: DailyReportBilling},
		{Start: finishDay, Kind: DailyReportUsage},
		{Start: refundDay - billingReportDaySeconds, Kind: DailyReportBilling},
		{Start: refundDay, Kind: DailyReportBilling},
		{Start: refundDay, Kind: DailyReportUsage},
	}, scopes)
}

func TestDailyReportCloseReplacesProvisionalStage(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := StageDailyReport(t.Context())
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	var provisional dailyReportStage
	require.NoError(t, db.Where("start = ? AND kind = ?", start, scope.Kind).Take(&provisional).Error)
	require.NoError(t, db.Model(&provisional).Update("as_of", start+billingReportDaySeconds-1).Error)
	ready, err := GetDailyReportStage(ctx, scope, 0)
	require.NoError(t, err)
	require.False(t, ready)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, PublishScheduledDailyReport(ctx, []DailyReportScope{scope}, "billing-test", "runner", start, nil))
	_, report, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	assert.Equal(t, 1, report.PublishedDays)
	assert.NotEqual(t, provisional.Generation, report.Version)
}

func TestDailyReportAcknowledgesOnlyCoveredCorrectionRevisions(t *testing.T) {
	db, primary, _ := setupBillingReportTest(t)
	ctx := StageDailyReport(t.Context())
	related, partial := primary+2*billingReportDaySeconds, primary+3*billingReportDaySeconds
	for _, day := range []int64{primary, related, partial} {
		require.NoError(t, db.Create(&billingReportRebuild{Start: day, RequestedAt: 1}).Error)
		require.NoError(t, db.Create(&[]Log{
			{UserId: 7, TokenId: 11, ModelName: "text", CreatedAt: day + 1, Type: LogTypeConsume, Quota: 100, Other: `{"contract_applicable":false,"model_ratio":1,"group_ratio":1}`},
			{UserId: 7, TokenId: 11, ModelName: "text", CreatedAt: day + 2, Type: LogTypeRefund, Quota: 30, Other: `{"contract_applicable":false,"model_ratio":1,"group_ratio":1}`},
		}).Error)
	}
	scopes := []DailyReportScope{
		{Start: primary, Kind: DailyReportBilling}, {Start: primary, Kind: DailyReportUsage},
		{Start: related, Kind: DailyReportBilling}, {Start: related, Kind: DailyReportUsage},
		{Start: partial, Kind: DailyReportUsage},
	}
	revisions, err := DailyReportRebuildRevisions(ctx, scopes)
	require.NoError(t, err)
	for _, scope := range scopes {
		if scope.Kind == DailyReportBilling {
			require.NoError(t, BuildBillingReportDay(ctx, scope.Start, "billing-test", "runner", nil, nil))
		} else {
			date := time.Unix(scope.Start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
			require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
		}
	}
	require.NoError(t, db.Model(&billingReportRebuild{}).Where("start = ?", primary).Update("requested_at", 2).Error)

	require.NoError(t, PublishScheduledDailyReport(ctx, scopes, "billing-test", "runner", primary, revisions))
	var pending []billingReportRebuild
	require.NoError(t, db.Order("start").Find(&pending).Error)
	assert.Equal(t, []billingReportRebuild{{Start: primary, RequestedAt: 2}, {Start: partial, RequestedAt: 1}}, pending)
	for _, day := range []int64{primary, related} {
		pinned, report, err := ResolveBillingReport(ctx, day, day+billingReportDaySeconds-1, "")
		require.NoError(t, err)
		require.Equal(t, 1, report.PublishedDays)
		statement, err := GetBillingCustomerStatement(pinned, 7, day, day+billingReportDaySeconds-1, "api_key", 0, "", "")
		require.NoError(t, err)
		assert.EqualValues(t, 70, statement.Summary.NetQuota)
		date := time.Unix(day, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
		period, err := ResolveUsageAnalyticsPeriod("day", date, common.GetTimestamp())
		require.NoError(t, err)
		usage, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
		require.NoError(t, err)
		require.Equal(t, []string{date}, meta.PublishedDays)
		assert.EqualValues(t, 100, usage.Total.NetQuota)
	}
}
