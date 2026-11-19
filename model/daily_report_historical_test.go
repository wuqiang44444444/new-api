package model

import (
	"context"
	"fmt"

	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistoricalDailyReportMigratesEvidenceWithoutPublishingMissingDaysAsZero(t *testing.T) {
	for _, withEvidence := range []bool{true, false} {
		name := "no historical evidence"
		if withEvidence {
			name = "frozen line items and summaries"
		}
		t.Run(name, func(t *testing.T) {
			db, start, end := setupBillingReportTest(t)
			ctx := context.Background()
			day := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
			if withEvidence {
				require.NoError(t, db.Create(&Log{
					UserId: 7, TokenId: 3, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "image", Quota: 300,
					Other: `{"billing_unit":"request","image_count":1,"request_path":"/v1/images/generations"}`,
				}).Error)
			}
			require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
			require.NoError(t, BuildUsageReportDay(ctx, day, "billing-test", "runner", nil))
			require.NoError(t, db.Model(&billingReportGeneration{}).Where("start = ?", start).
				Updates(map[string]any{"original_replay_version": CustomerOriginalReplayVersion - 1, "incomplete": true}).Error)
			require.NoError(t, db.Model(&usageReportDay{}).Where("start = ?", start).
				Updates(map[string]any{"meter_version": usageReportMeterVersion - 1, "incomplete": true}).Error)
			require.NoError(t, db.Where("created_at >= ? AND created_at <= ?", start, end).Delete(&Log{}).Error)

			_, prior, err := ResolveBillingReport(ctx, start, end, "")
			require.NoError(t, err)
			assert.Zero(t, prior.PublishedDays)
			assert.Equal(t, []string{day}, prior.RebuildingDays)
			period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, day, time.Now().Unix())
			require.NoError(t, err)
			_, pending, err := GetPublishedUsageCustomer(ctx, period, 7, "")
			require.NoError(t, err)
			assert.Equal(t, []string{day}, pending.RebuildingDays)
			assert.Empty(t, pending.References)
			for range 2 {
				for _, kind := range []string{DailyReportBilling, DailyReportUsage} {
					done, err := MigrateHistoricalDailyReport(ctx, DailyReportScope{Start: start, Kind: kind}, "billing-test", "runner")
					require.NoError(t, err)
					require.True(t, done)
				}
			}
			bound, manifest, err := ResolveBillingReport(ctx, start, end, "")
			require.NoError(t, err)
			usage, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
			require.NoError(t, err)
			mail, err := PublishedUsageMailDay(ctx, period)
			require.NoError(t, err)
			assert.Empty(t, mail.References)
			if !withEvidence {
				assert.Zero(t, manifest.PublishedDays)
				assert.Equal(t, []string{day}, manifest.SourceUnavailableDays)
				var head billingReportHead
				require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
				_, _, err = ResolveBillingReport(ctx, start, end, head.Generation)
				require.Error(t, err, "an explicit snapshot cannot silently omit an unavailable date")
				assert.Empty(t, meta.PublishedDays)
				assert.Equal(t, []string{day}, meta.SourceUnavailableDays)
				require.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportCoverageDecreased)
				require.NoError(t, db.Create(&Log{UserId: 7, TokenId: 3, CreatedAt: start + 10, Type: LogTypeConsume, ModelName: "restored", Quota: 123}).Error)
				require.NoError(t, BuildUsageReportDay(ctx, day, "billing-test", "runner", nil))
				require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
				restored, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
				require.NoError(t, err)
				assert.Empty(t, meta.SourceUnavailableDays)
				assert.EqualValues(t, 123, restored.Total.NetQuota)
				bound, manifest, err = ResolveBillingReport(ctx, start, end, "")
				require.NoError(t, err)
				assert.Equal(t, 1, manifest.PublishedDays)
				statement, err := GetBillingCustomerStatement(bound, 7, start, end, "api_key", 0, "", "")
				require.NoError(t, err)
				assert.EqualValues(t, 123, statement.Summary.NetQuota)
				return
			}
			assert.Equal(t, 1, manifest.IncompleteDays)
			assert.Equal(t, []string{day}, meta.IncompleteDays)
			assert.EqualValues(t, 300, usage.Total.NetQuota)
			statement, err := GetBillingCustomerStatement(bound, 7, start, end, "api_key", 0, "", "")
			require.NoError(t, err)
			assert.EqualValues(t, 300, statement.Summary.NetQuota)
		})
	}
}

func TestHistoricalUsageSeparatesRefundNoticesFromUsageEvidence(t *testing.T) {
	for _, evidence := range []string{"refund only", "legacy customer leaves", "upstream leaves", "customer overview"} {
		t.Run(evidence, func(t *testing.T) {
			db, start, _ := setupBillingReportTest(t)
			ctx := context.Background()
			day := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
			period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, day, time.Now().Unix())
			require.NoError(t, err)

			const previous = "previous-usage"
			require.NoError(t, db.Create(&usageReportDay{Start: start, Generation: previous,
				MeterVersion: usageReportMeterVersion - 1, HistoricalMeterVersion: 7,
				HistoricalSource: dailyReportSourceSummary, Incomplete: true, AsOf: start + 2*billingReportDaySeconds}).Error)
			refund := PublishedUsagePendingRefund{TaskID: "pending-7", RequestedAt: start + 1}
			notice, err := common.Marshal(usageReportDayData{PendingRefunds: []PublishedUsagePendingRefund{refund}})
			require.NoError(t, err)
			// The notice is chunked, as are some older customer detail projections.
			require.NoError(t, db.Create(&usageReportDayRow{Generation: previous, UserID: 7, Overview: "{}"}).Error)
			require.NoError(t, db.Create(&dailyReportPayloadPart{Generation: previous, Kind: reportUsagePayload, ScopeID: 7, Payload: BillingStatementJSON(notice)}).Error)
			switch evidence {
			case "legacy customer leaves":
				require.NoError(t, db.Create(&usageReportDayRow{Generation: previous, UserID: 8}).Error)
				require.NoError(t, db.Create(&dailyReportPayloadPart{Generation: previous, Kind: reportUsagePayload, ScopeID: 8, Payload: `{"Customers":[{"Model":"img","Metrics":{"Metrics":{"total_calls":3}}}]}`}).Error)
			case "upstream leaves":
				require.NoError(t, db.Create(&usageReportDayRow{Generation: previous, UserID: 0, Payload: `{"Upstream":[{"Channel":12}]}`}).Error)
			case "customer overview":
				require.NoError(t, db.Create(&usageReportDayRow{Generation: previous, UserID: 8, Payload: "{}", Overview: `{"Metrics":{"total_calls":3}}`}).Error)
			}
			_, stale, err := GetPublishedUsageCustomer(ctx, period, 7, "")
			require.NoError(t, err)
			assert.Equal(t, []string{day}, stale.RebuildingDays)
			assert.Empty(t, stale.References)
			for range 2 {
				done, err := MigrateHistoricalDailyReport(ctx, DailyReportScope{Start: start, Kind: DailyReportUsage}, "billing-test", "runner")
				require.NoError(t, err)
				require.True(t, done)
				_, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
				require.NoError(t, err)
				assert.Equal(t, []PublishedUsagePendingRefund{refund}, meta.PendingRefunds)
				other, otherMeta, err := GetPublishedUsageCustomer(ctx, period, 8, "")
				require.NoError(t, err)
				assert.Empty(t, otherMeta.PendingRefunds)
				if evidence == "refund only" {
					assert.Equal(t, []string{day}, meta.SourceUnavailableDays)
					assert.Empty(t, meta.PublishedDays)
					assert.Empty(t, meta.References)
					_, err = FreezeUsageReportExport(ctx, period, nil)
					assert.Error(t, err)
					var head usageReportDay
					require.NoError(t, db.Where("start = ?", start).Take(&head).Error)
					_, err = FreezeUsageReportReferences(ctx, period, []UsageReportDayReference{{Start: start, Generation: head.Generation}})
					assert.Error(t, err)
					mail, err := PublishedUsageMailDay(ctx, period)
					require.NoError(t, err)
					assert.Empty(t, mail.References)
				} else {
					assert.Equal(t, []string{day}, meta.PublishedDays)
					assert.Equal(t, []string{day}, meta.IncompleteDays)
					if evidence == "legacy customer leaves" || evidence == "customer overview" {
						assert.EqualValues(t, 3, other.Total.TotalCalls)
					}
				}
			}
		})
	}
}

func TestHistoricalUsageKeepsCoverageThroughMigrationAndPartialRestore(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, date, time.Now().Unix())
	require.NoError(t, err)
	logs := []Log{
		{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "demo", Quota: 20},
		{UserId: 7, CreatedAt: start + 2, Type: LogTypeConsume, ModelName: "demo", Quota: 30},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	require.NoError(t, db.Where("created_at >= ? AND created_at <= ?", start, end).Delete(&Log{}).Error)
	scope := DailyReportScope{Start: start, Kind: DailyReportUsage}
	done, err := MigrateHistoricalDailyReport(ctx, scope, "billing-test", "runner")
	require.NoError(t, err)
	require.False(t, done, "a complete current generation cannot be downgraded")
	require.NoError(t, db.Model(&usageReportDay{}).Where("start = ?", start).Update("meter_version", usageReportMeterVersion-1).Error)
	done, err = MigrateHistoricalDailyReport(ctx, scope, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, done)
	require.NoError(t, db.Create(&logs[1]).Error)
	require.ErrorIs(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil), errDailyReportCoverageDecreased)
	view, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 50, view.Total.NetQuota)
	assert.Equal(t, []string{date}, meta.IncompleteDays)

	require.NoError(t, db.Create(&logs[0]).Error)
	require.NoError(t, db.Create(&SystemTask{
		Type: SystemTaskTypeLogCleanup, TaskID: "review-cleanup", Status: SystemTaskStatusSucceeded,
		Payload: fmt.Sprintf(`{"target_timestamp":%d}`, end+1),
	}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	view, meta, err = GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 50, view.Total.NetQuota)
	assert.Empty(t, meta.IncompleteDays)
}

func TestHistoricalBillingReconstructsMissingCustomerSummaryFromFrozenLines(t *testing.T) {
	db, start, end := setupBillingReportTest(t)
	ctx := context.Background()
	logs := []Log{
		{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "image", Quota: 100},
		{UserId: 7, CreatedAt: start + 2, Type: LogTypeConsume, ModelName: "image", Quota: 200},
	}
	require.NoError(t, db.Create(&logs).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	require.NoError(t, db.Where("user_id = ?", 7).Delete(&billingReportCustomerDay{}).Error)
	require.NoError(t, db.Delete(&logs[0]).Error)
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("start = ?", start).
		Updates(map[string]any{"original_replay_version": CustomerOriginalReplayVersion - 1,
			"rows": 0, "historical_source": dailyReportSourceDetails}).Error)
	done, err := MigrateHistoricalDailyReport(ctx, DailyReportScope{Start: start, Kind: DailyReportBilling}, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, done)
	bound, manifest, err := ResolveBillingReport(ctx, start, end, "")
	require.NoError(t, err)
	require.Equal(t, 1, manifest.PublishedDays)
	statement, err := GetBillingCustomerStatement(bound, 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 300, statement.Summary.NetQuota)
}

func TestHistoricalSummaryWithoutOriginalBoundaryStillUpgradesWithResidualLogs(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod(UsageAnalyticsPeriodDay, date, time.Now().Unix())
	require.NoError(t, err)
	require.NoError(t, db.Create(&usageReportDay{Start: start, Generation: "previous-summary", MeterVersion: usageReportMeterVersion - 1,
		HistoricalSource: dailyReportSourceSummary, Incomplete: true, AsOf: start + 2*billingReportDaySeconds}).Error)
	require.NoError(t, db.Create(&usageReportDayRow{Generation: "previous-summary", UserID: 7, Overview: `{"Metrics":{"total_calls":3}}`}).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "restored", Quota: 5}).Error)
	done, err := MigrateHistoricalDailyReport(ctx, DailyReportScope{Start: start, Kind: DailyReportUsage}, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, done)
	view, meta, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.EqualValues(t, 3, view.Total.TotalCalls)
	assert.Equal(t, []string{date}, meta.IncompleteDays)
	assert.Empty(t, meta.RebuildingDays)
}

func TestPartialLogRetentionPreservesFundingUntilSourcesRestored(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	scope := DailyReportScope{Start: start, Kind: DailyReportBilling}
	logs := make([]Log, 100)
	for i := range logs {
		logs[i] = Log{UserId: 7, Type: LogTypeConsume, CreatedAt: start + int64(i+1), ModelName: "image", Quota: 10}
	}
	logs[0].UserId = 0
	require.NoError(t, db.CreateInBatches(&logs, 25).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	var before billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&before).Error)
	require.NoError(t, db.Model(&billingReportGeneration{}).Where("id = ?", before.Generation).Update("original_replay_version", CustomerOriginalReplayVersion-1).Error)
	require.NoError(t, db.Delete(logs[:20]).Error)
	require.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportCoverageDecreased)
	days, err := BillingReportDiagnosticDays(ctx, start, start+billingReportDaySeconds)
	require.NoError(t, err)
	require.Len(t, days, 1)
	require.Equal(t, "coverage_issue", days[0].Status)
	require.NotNil(t, days[0].PublishedRows)
	require.NotNil(t, days[0].CandidateRows)
	assert.EqualValues(t, 100, *days[0].PublishedRows)
	assert.EqualValues(t, 80, *days[0].CandidateRows)
	_, pending, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	assert.Len(t, pending.RebuildingDays, 1)
	assert.Empty(t, pending.SourceUnavailableDays)
	migrated, err := MigrateHistoricalDailyReport(ctx, scope, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, migrated, "publish only the stronger frozen evidence into the current version")
	var preserved billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&preserved).Error)
	_, frozen, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	assert.Equal(t, 1, frozen.PublishedDays)
	assert.Equal(t, 1, frozen.IncompleteDays)
	require.NoError(t, db.CreateInBatches(logs[:20], 20).Error)
	// Equal counts cannot conceal a change in a non-customer funding event.
	require.NoError(t, db.Model(&logs[0]).Update("channel_id", 11).Error)
	require.ErrorIs(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil), errDailyReportCoverageDecreased)
	var unchanged billingReportHead
	require.NoError(t, db.Where("start = ?", start).Take(&unchanged).Error)
	assert.Equal(t, preserved.Generation, unchanged.Generation)
	require.NoError(t, db.Model(&logs[0]).Update("channel_id", 0).Error)
	require.NoError(t, BuildBillingReportDay(ctx, start, "billing-test", "runner", nil, nil))
	pinned, meta, err := ResolveBillingReport(ctx, start, start+billingReportDaySeconds-1, "")
	require.NoError(t, err)
	require.Equal(t, 1, meta.PublishedDays)
	view, err := GetBillingCustomerStatement(pinned, 7, start, start+billingReportDaySeconds-1, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 990, view.Summary.NetQuota)
}

func TestHistoricalUsageRefreshesRefundNoticesFromRetainedTasks(t *testing.T) {
	db, start, _ := setupBillingReportTest(t)
	ctx := context.Background()
	now := common.GetTimestamp()
	date := time.Unix(start, 0).In(UsageAnalyticsZone()).Format(time.DateOnly)
	period, err := ResolveUsageAnalyticsPeriod("day", date, now)
	require.NoError(t, err)
	task := Task{TaskID: "fixture-refund", UserId: 7, Status: TaskStatusSuccess, FinishTime: start + 2,
		VideoRefund: VideoRefund{VideoRefundState: "pending", VideoRefundRequestedAt: now - 86401}}
	require.NoError(t, db.Create(&task).Error)
	require.NoError(t, db.Create(&Log{UserId: 7, CreatedAt: start + 1, Type: LogTypeConsume, ModelName: "demo", Quota: 30}).Error)
	require.NoError(t, BuildUsageReportDay(ctx, date, "billing-test", "runner", nil))
	_, initial, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	require.Len(t, initial.PendingRefunds, 1)
	require.NoError(t, db.Model(&usageReportDay{}).Where("start = ?", start).Update("meter_version", usageReportMeterVersion-1).Error)
	require.NoError(t, db.Where("user_id = ?", 7).Delete(&Log{}).Error)
	require.NoError(t, db.Model(&task).Updates(VideoRefund{VideoRefundState: "refunded", VideoRefundCompletedAt: now}).Error)
	done, err := MigrateHistoricalDailyReport(ctx, DailyReportScope{Start: start, Kind: DailyReportUsage}, "billing-test", "runner")
	require.NoError(t, err)
	require.True(t, done)
	view, migrated, err := GetPublishedUsageCustomer(ctx, period, 7, "")
	require.NoError(t, err)
	assert.Empty(t, migrated.PendingRefunds)
	assert.EqualValues(t, 30, view.Total.NetQuota)
}
