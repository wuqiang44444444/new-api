package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDailyReportIsScheduledByDefaultForRelationalLogs(t *testing.T) {
	setupVersionServiceTestDB(t)
	main, logs := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { common.SetDatabaseTypes(main, logs) })
	common.SetDatabaseTypes(main, common.DatabaseTypeSQLite)
	require.True(t, (usageReportHandler{}).Enabled())
	common.SetDatabaseTypes(main, common.DatabaseTypeClickHouse)
	require.False(t, (usageReportHandler{}).Enabled())
}

func TestReportForegroundPressureTracksTrafficAndLogCommit(t *testing.T) {
	now := time.Now()
	var window reportForegroundWindow
	var snapshot common.ReportForegroundSnapshot
	require.Equal(t, reportReduce, window.observe(now, snapshot, true))
	snapshot.Operations[common.ReportForegroundLog] = common.ReportForegroundCounters{Completed: 20, Milliseconds: 200}
	require.Equal(t, reportHealthy, window.observe(now.Add(time.Second), snapshot, true))
	snapshot.Active = 40
	snapshot.Operations[common.ReportForegroundLog] = common.ReportForegroundCounters{Completed: 40, Milliseconds: 510}
	assert.Equal(t, reportReduce, window.observe(now.Add(2*time.Second), snapshot, true))
	snapshot.Operations[common.ReportForegroundLog].Failed++
	assert.Equal(t, reportPause, window.observe(now.Add(3*time.Second), snapshot, true))
	assert.Equal(t, reportPause, window.observe(now.Add(3*time.Second), snapshot, true), "cached samples retain their pressure")
}

func TestReportForegroundLogTailSurfacesPressureWithStableRequestCount(t *testing.T) {
	now := time.Now()
	var window reportForegroundWindow
	var snapshot common.ReportForegroundSnapshot
	window.observe(now, snapshot, false)
	logs := &snapshot.Operations[common.ReportForegroundLog]
	logs.Completed, logs.Milliseconds, logs.Latency[2] = 100, 400, 100
	require.Equal(t, reportHealthy, window.observe(now.Add(time.Second), snapshot, true))
	require.Equal(t, 4*time.Millisecond, window.tail95[common.ReportForegroundLog])
	logs.Completed, logs.Milliseconds, logs.Latency[2], logs.Latency[5] = 200, 950, 194, 6
	require.Equal(t, reportPause, window.observe(now.Add(2*time.Second), snapshot, false))
	require.Equal(t, 4*time.Millisecond, window.tail95[common.ReportForegroundLog], "the background slowdown cannot raise the reference")
}

func TestReportForegroundTailReducesWorkBeforeTheAverageRises(t *testing.T) {
	for _, operation := range []common.ReportForegroundOperation{common.ReportForegroundRequest, common.ReportForegroundSettlement} {
		now := time.Now()
		var window reportForegroundWindow
		var snapshot common.ReportForegroundSnapshot
		window.observe(now, snapshot, false)
		counter := &snapshot.Operations[operation]
		counter.Completed, counter.Milliseconds, counter.Latency[0] = 34, 34, 34
		require.Equal(t, reportHealthy, window.observe(now.Add(time.Second), snapshot, true))
		counter.Completed, counter.Milliseconds, counter.Latency[0], counter.Latency[3] = 68, 74, 66, 2
		require.GreaterOrEqual(t, window.observe(now.Add(2*time.Second), snapshot, false), reportReduce)
	}
}

func TestReportRatesRampOncePerWindowAndRestartConservatively(t *testing.T) {
	var speed adaptiveReportRate
	now := time.Now()
	advance := func(seconds int, pressure reportPressure) int {
		return speed.advance(reportAdmissionDecision{pressure: pressure, at: now.Add(time.Duration(seconds) * time.Second)}, 1000, 16000)
	}
	assert.Equal(t, 1000, advance(0, reportHealthy))
	assert.Equal(t, 1000, advance(1, reportHealthy))
	assert.Equal(t, 1000, advance(2, reportHealthy))
	assert.Equal(t, 1250, advance(3, reportHealthy))
	assert.Equal(t, 1250, advance(3, reportHealthy))
	assert.Equal(t, 1000, advance(4, reportReduce))
	assert.Equal(t, 1000, advance(5, reportPause))
	assert.Equal(t, 1000, advance(30, reportHealthy))
}

func TestDailyReportDefersHistoricalWritesUnderSharedPressure(t *testing.T) {
	ctx := t.Context()
	now := time.Now()
	admission := reportAdmissionDecision{pressure: reportPause, at: now}
	budget := dailyReportBudget{
		admission: func(context.Context) reportAdmissionDecision { return admission },
	}
	require.ErrorIs(t, budget.pace(ctx, 100), model.ErrDailyReportResourcePressure)
	require.Zero(t, budget.nextWriteAt)
	admission = reportAdmissionDecision{pressure: reportHealthy, at: now.Add(reportObservationInterval)}
	require.NoError(t, budget.pace(ctx, 100))
	require.False(t, budget.nextWriteAt.IsZero())
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, budget.pace(cancelled, 100), context.Canceled)
}

func TestDailyReportValidationYieldsAndRecentDeadlineKeepsPressureReason(t *testing.T) {
	budget := dailyReportBudget{pressure: true, admission: func(context.Context) reportAdmissionDecision {
		return reportAdmissionDecision{pressure: reportPause, at: time.Now()}
	}}
	require.ErrorIs(t, budget.validationGate(t.Context()), model.ErrDailyReportResourcePressure)

	expired, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	require.ErrorIs(t, budget.gate(expired), model.ErrDailyReportResourcePressure)
	budget.pressure = false
	require.ErrorIs(t, budget.gate(expired), context.DeadlineExceeded)
}

func TestDailyReportCleanupDefersDuringCalibration(t *testing.T) {
	decision := reportAdmissionDecision{calibrating: true, pressure: reportPause, at: time.Now()}
	budget := dailyReportBudget{admission: func(context.Context) reportAdmissionDecision {
		current := decision
		decision = reportAdmissionDecision{pressure: reportHealthy, at: current.at.Add(reportObservationInterval)}
		return current
	}}
	ctx := context.WithValue(t.Context(), dailyReportCleanupWorkKey{}, true)
	require.ErrorIs(t, budget.pace(ctx, 25), model.ErrDailyReportResourcePressure)
}

func TestSharedAdmissionStopsAtWALPressure(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
	now := time.Now()
	sharedReportAdmission.Lock()
	sharedReportAdmission.db = model.DB
	sharedReportAdmission.decision = reportAdmissionDecision{at: now.Add(-reportObservationInterval)}
	sharedReportAdmission.lastQuiet = now
	sharedReportAdmission.quietAt = now.Add(-time.Minute)
	sharedReportAdmission.last = []model.DailyReportDatabaseLoad{{WALBytes: 1}}
	sharedReportAdmission.Unlock()
	decision := checkReportAdmission(t.Context(), func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
		return []model.DailyReportDatabaseLoad{{WALBytes: 1 + 32<<20}}, nil
	})
	assert.Equal(t, reportPause, decision.pressure)
}

func TestSharedAdmissionStopsForAnotherNodesLogFailure(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
	now := time.Now().Truncate(time.Second)
	node := "foreground-peer"
	var before, after common.ReportForegroundSnapshot
	after.Operations[common.ReportForegroundLog] = common.ReportForegroundCounters{Completed: 100, Failed: 1}
	require.NoError(t, model.UpsertSystemInstance(node, map[string]any{"report_foreground": after},
		now.Add(-time.Minute).Unix(), now.Unix()))
	sharedReportAdmission.Lock()
	sharedReportAdmission.db = model.DB
	sharedReportAdmission.lastQuiet = now
	sharedReportAdmission.quietAt = now.Add(-time.Minute)
	sharedReportAdmission.peerAt = time.Time{}
	sharedReportAdmission.peers = map[string]reportForegroundWindow{
		node: {at: now.Add(-5 * time.Second), snapshot: before},
	}
	sharedReportAdmission.decision = reportAdmissionDecision{at: now.Add(-2 * reportObservationInterval)}
	sharedReportAdmission.last = []model.DailyReportDatabaseLoad{{}}
	sharedReportAdmission.Unlock()
	decision := checkReportAdmission(t.Context(), func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
		return []model.DailyReportDatabaseLoad{{}}, nil
	})
	assert.Equal(t, reportPause, decision.pressure)
}

func TestReportAdmissionKeepsCalibrationSeparateFromForegroundPressure(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
	for _, calibrating := range []bool{true, false} {
		now := time.Now()
		prior := now.Add(-reportObservationInterval)
		sharedReportAdmission.Lock()
		sharedReportAdmission.db = model.DB
		sharedReportAdmission.decision = reportAdmissionDecision{pressure: reportPause, at: prior, calibrating: calibrating}
		sharedReportAdmission.lastQuiet = now
		sharedReportAdmission.quietAt = now.Add(-time.Minute)
		sharedReportAdmission.last = []model.DailyReportDatabaseLoad{{}}
		sharedReportAdmission.local = reportForegroundWindow{at: prior, snapshot: common.CurrentReportForeground()}
		sharedReportAdmission.healthy = 0
		sharedReportAdmission.peerPressure = reportHealthy
		sharedReportAdmission.Unlock()
		decision := checkReportAdmission(t.Context(), func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
			return []model.DailyReportDatabaseLoad{{}}, nil
		})
		assert.Equal(t, reportPause, decision.pressure)
		assert.Equal(t, calibrating, decision.calibrating)
	}
}

func TestReportAdmissionRecoveryNeedsConsecutiveHealthySamples(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemInstance{}))
	now := time.Now()
	prior := now.Add(-reportObservationInterval)
	sharedReportAdmission.Lock()
	sharedReportAdmission.db = model.DB
	sharedReportAdmission.decision = reportAdmissionDecision{pressure: reportPause, at: prior}
	sharedReportAdmission.local = reportForegroundWindow{at: prior, snapshot: common.CurrentReportForeground()}
	sharedReportAdmission.last = []model.DailyReportDatabaseLoad{{}}
	sharedReportAdmission.lastQuiet, sharedReportAdmission.quietAt = now, time.Time{}
	sharedReportAdmission.peerAt, sharedReportAdmission.peerPressure = now, reportHealthy
	sharedReportAdmission.healthy = 2
	sharedReportAdmission.Unlock()

	decision := checkReportAdmission(t.Context(), func(context.Context) ([]model.DailyReportDatabaseLoad, error) {
		return []model.DailyReportDatabaseLoad{{Probe: 40 * time.Millisecond}}, nil
	})
	assert.Equal(t, reportPause, decision.pressure)
}
