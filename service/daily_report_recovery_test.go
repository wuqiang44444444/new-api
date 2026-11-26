package service

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduledDailyReportPublishesAfterResourcePressureClears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := setupVersionServiceTestDB(t)
		require.NoError(t, db.AutoMigrate(&model.SystemInstance{}, &model.TaskBillingDelivery{}, &model.SystemTask{}, &model.SystemTaskLock{}))
		prepareDailyReportExportTest(t)
		// The test invokes the worker once per scheduled pass. The real runner renews its lease.
		task := &model.SystemTask{TaskID: "export-fixture"}
		const runner = "runner"
		require.NoError(t, db.Model(&model.SystemTaskLock{}).Where("type = ?", model.UsageReportTaskType).
			Update("locked_until", time.Now().Add(10*time.Minute).Unix()).Error)
		now := time.Now().In(model.UsageAnalyticsZone())
		start := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, now.Location()).Unix()
		end := start + 86400 - 1
		require.NoError(t, db.Create(&model.Log{
			UserId: 7, ModelName: "fixture", Type: model.LogTypeConsume, CreatedAt: start + 1, Quota: 300,
		}).Error)

		probe := 110 * time.Millisecond
		run := func() {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			budget := dailyReportBudget{admission: func(ctx context.Context) reportAdmissionDecision {
				return checkReportAdmission(ctx, func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
					return []model.DailyReportDatabaseLoad{{Probe: probe}}, nil
				})
			}}
			ctx = model.WithDailyReportWorkerBudget(ctx, budget.gate, budget.pace)
			ctx = model.WithDailyReportValidationGate(ctx, budget.validationGate)
			require.NoError(t, runDailyReport(ctx, task, runner, budget.gate))
			require.NoError(t, ctx.Err(), "a scheduled pass must release the worker under ongoing pressure")
		}
		run()
		// synctest advances the scheduled interval on its virtual clock.
		time.Sleep(usageReportHandler{}.Interval())
		run()
		time.Sleep(usageReportHandler{}.Interval())
		probe = 60 * time.Millisecond
		logs := captureDailyReportLogs(t)
		run()
		assert.Contains(t, logs.String(), "daily report recovery wait expired | source=admission trigger=recovery_healthy_samples sample=0 threshold=3 | source=main_db trigger=db_probe_latency sample=60ms threshold=55ms")
		_, paused, err := model.ResolveBillingReport(t.Context(), start, end, "")
		require.NoError(t, err)
		require.Zero(t, paused.PublishedDays)

		time.Sleep(usageReportHandler{}.Interval())
		probe = time.Millisecond
		run()

		bound, recovered, err := model.ResolveBillingReport(t.Context(), start, end, "")
		require.NoError(t, err)
		require.Equal(t, 1, recovered.PublishedDays)
		statement, err := model.GetBillingCustomerStatement(bound, 7, start, end, "api_key", 0, "", "")
		require.NoError(t, err)
		assert.EqualValues(t, 300, statement.Summary.NetQuota)
	})
}

func TestDailyReportWaitsForRecoveryOnlyOncePerPass(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		budget := dailyReportBudget{admission: func(context.Context) reportAdmissionDecision {
			return reportAdmissionDecision{pressure: reportPause, recovering: true, at: time.Now()}
		}}
		ctx := t.Context()
		cleanup := context.WithValue(ctx, dailyReportCleanupWorkKey{}, true)
		start := time.Now()

		require.ErrorIs(t, budget.gate(cleanup), model.ErrDailyReportResourcePressure)
		assert.Zero(t, time.Since(start))
		require.ErrorIs(t, budget.gate(ctx), model.ErrDailyReportResourcePressure)
		assert.Equal(t, 10*time.Second, time.Since(start))
		require.ErrorIs(t, budget.gate(ctx), model.ErrDailyReportResourcePressure)
		assert.Equal(t, 10*time.Second, time.Since(start))
	})
}

func TestDailyReportRecoveryDeadlineBoundsSamplingAndCalibration(t *testing.T) {
	for _, phase := range []string{"sampling", "calibration"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
				defer cancel()
				start := time.Now()
				sampled := false
				budget := dailyReportBudget{admission: func(ctx context.Context) reportAdmissionDecision {
					now := time.Now()
					if !sampled {
						sampled = true
						return reportAdmissionDecision{pressure: reportPause, recovering: true, at: now}
					}
					if phase == "sampling" {
						<-ctx.Done()
						return reportAdmissionDecision{pressure: reportHealthy, at: now}
					}
					assert.Less(t, now.Sub(start), 10*time.Second, "no sample should start at or after the deadline")
					return reportAdmissionDecision{pressure: reportPause, calibrating: true, at: now}
				}}

				require.ErrorIs(t, budget.gate(ctx), model.ErrDailyReportResourcePressure)
				assert.Equal(t, 10*time.Second, time.Since(start))
				require.NoError(t, ctx.Err(), "the recovery deadline must be independent of the parent")
			})
		})
	}
}

func TestDailyReportCleanupChecksPressureBeforeWritePacing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		budget := dailyReportBudget{
			nextWriteAt: start.Add(time.Second),
			admission: func(context.Context) reportAdmissionDecision {
				return reportAdmissionDecision{pressure: reportPause, at: time.Now()}
			},
		}
		ctx := context.WithValue(t.Context(), dailyReportCleanupWorkKey{}, true)

		require.ErrorIs(t, budget.pace(ctx, 25), model.ErrDailyReportResourcePressure)
		assert.Zero(t, time.Since(start))
	})
}
