package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm"
)

const (
	reportObservationInterval = time.Second
	reportBaselineInterval    = time.Minute
	reportBaselineQuietWindow = 6 * time.Second
	reportBaselineDrainTime   = 2 * time.Second
	reportRecoverySamples     = 3
)

type reportPressure uint8

const (
	reportHealthy reportPressure = iota
	reportReduce
	reportPause
)

type reportAdmissionDecision struct {
	calibrating bool
	recovering  bool
	pressure    reportPressure
	at          time.Time
	reasons     []string
}

// Keep the triggers at the highest observed pressure. A recovery hold also
// records the reduction signals that prevent a healthy sample.
type reportPressureObservation struct {
	level   reportPressure
	reasons []string
}

func (p *reportPressureObservation) record(level reportPressure, format string, args ...any) {
	if level < p.level {
		return
	}
	if level > p.level {
		p.level, p.reasons = level, nil
	}
	p.reasons = append(p.reasons, fmt.Sprintf(format, args...))
}

func (p *reportPressureObservation) merge(prefix string, other reportPressureObservation) {
	if other.level < p.level {
		return
	}
	if other.level > p.level {
		p.level, p.reasons = other.level, nil
	}
	for _, reason := range other.reasons {
		p.reasons = append(p.reasons, prefix+reason)
	}
}

func reportSampleErrorStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "error"
}

// One admission state observes the load of both report producers in this
// process. Cross-node foreground snapshots use the existing instance heartbeat.
var sharedReportAdmission = struct {
	lock         *semaphore.Weighted
	db           *gorm.DB
	decision     reportAdmissionDecision
	healthy      int
	last         []model.DailyReportDatabaseLoad
	local        reportForegroundWindow
	peers        map[string]reportForegroundWindow
	quietAt      time.Time
	lastQuiet    time.Time
	peerAt       time.Time
	peerPressure reportPressureObservation
}{lock: semaphore.NewWeighted(1)}

func checkReportAdmission(ctx context.Context, sample func(context.Context) ([]model.DailyReportDatabaseLoad, error)) reportAdmissionDecision {
	state := &sharedReportAdmission
	if err := state.lock.Acquire(ctx, 1); err != nil {
		return reportAdmissionDecision{pressure: reportPause, at: time.Now()}
	}
	var probeFailure string
	defer func() {
		state.lock.Release(1)
		// A probe failure is logged once even when its caller is already paused or canceled.
		if probeFailure != "" {
			common.SysLog("report database probe failed | " + probeFailure)
		}
	}()
	if state.db != model.DB {
		state.db = model.DB
		state.decision = reportAdmissionDecision{}
		state.healthy, state.last, state.local, state.peers = 0, nil, reportForegroundWindow{}, nil
		state.quietAt, state.lastQuiet, state.peerAt = time.Time{}, time.Time{}, time.Time{}
		state.peerPressure = reportPressureObservation{}
	}

	now := time.Now()
	if ctx.Err() != nil {
		return reportAdmissionDecision{pressure: reportPause, at: now}
	}
	previous := state.decision
	elapsed := now.Sub(previous.at)
	if elapsed < reportObservationInterval {
		return previous
	}
	quietAt := state.quietAt
	startQuiet := state.lastQuiet.IsZero() || (elapsed <= 4*reportObservationInterval && now.Sub(state.lastQuiet) >= reportBaselineInterval)
	if startQuiet {
		quietAt = now
	}
	quiet := now.Sub(quietAt) < reportBaselineQuietWindow
	learning := (quiet && now.Sub(quietAt) >= reportBaselineDrainTime) || (elapsed > 4*reportObservationInterval && reportArtifactUploads.Load() == 0)
	priorForeground := state.local
	currentForeground := common.CurrentReportForeground()
	state.local.observe(now, currentForeground, false)
	var pressure reportPressureObservation
	pressure.merge("source=local_foreground ", state.local.pressure)
	samples, err := sample(ctx)
	const minDatabaseSamples = 1
	switch {
	case err != nil:
		var probeErr *model.DailyReportProbeError
		if errors.As(err, &probeErr) {
			probeFailure = fmt.Sprintf("source=%s trigger=db_sample_error sample=%s elapsed=%s timeout=%s threshold=successful_sample",
				probeErr.Source, reportSampleErrorStatus(err), probeErr.Elapsed, probeErr.Timeout)
			pressure.record(reportPause, "%s", probeFailure)
		} else {
			pressure.record(reportPause, "source=database trigger=db_sample_error sample=%s threshold=successful_sample", reportSampleErrorStatus(err))
		}
	case len(samples) < minDatabaseSamples:
		pressure.record(reportPause, "source=database trigger=db_sample_empty sample=%d threshold=%d", len(samples), minDatabaseSamples)
	}
	if ctx.Err() != nil {
		state.local = priorForeground
		return reportAdmissionDecision{pressure: reportPause, at: now, reasons: pressure.reasons}
	}
	observeReportDatabasePressure(&pressure, samples, state.last, elapsed)
	if now.Sub(state.peerAt) >= 5*time.Second {
		const peerTimeout = 150 * time.Millisecond
		peerCtx, cancel := context.WithTimeout(ctx, peerTimeout)
		instances, err := model.ReportForegroundInstances(peerCtx, now.Unix())
		cancel()
		if ctx.Err() != nil {
			state.local = priorForeground
			return reportAdmissionDecision{pressure: reportPause, at: now, reasons: pressure.reasons}
		}
		state.peerAt = now
		state.peers, state.peerPressure = observeReportPeerPressure(now, instances, err, peerTimeout, state.peers, learning)
	}
	state.last = samples
	if startQuiet {
		state.quietAt, state.lastQuiet = quietAt, quietAt
	}
	pressure.merge("", state.peerPressure)
	level := pressure.level
	if learning && level == reportHealthy && reportArtifactUploads.Load() == 0 {
		priorForeground.observe(now, currentForeground, true)
		state.local = priorForeground
	}
	calibrating := quiet && level != reportPause
	if quiet {
		level = reportPause
	}

	if level == reportHealthy {
		state.healthy++
	} else {
		state.healthy = 0
	}
	recovering := level < reportPause && previous.pressure == reportPause && state.healthy < reportRecoverySamples
	if recovering {
		calibrating = previous.calibrating && level == reportHealthy
		level = reportPause
		recovery := fmt.Sprintf("source=admission trigger=recovery_healthy_samples sample=%d threshold=%d", state.healthy, reportRecoverySamples)
		pressure.reasons = append([]string{recovery}, pressure.reasons...)
	}
	state.decision = reportAdmissionDecision{pressure: level, at: now, calibrating: calibrating, recovering: recovering, reasons: pressure.reasons}
	return state.decision
}

type adaptiveReportRate struct {
	rate    int
	last    time.Time
	healthy int
}

// Workers share the observed state, while pacing their own read or write cost.
func (r *adaptiveReportRate) advance(d reportAdmissionDecision, minimum, maximum int) int {
	if r.rate == 0 || d.at.Sub(r.last) > 4*reportObservationInterval {
		r.rate, r.last, r.healthy = minimum, d.at, 0
	}
	if d.at.After(r.last) {
		r.last = d.at
		switch d.pressure {
		case reportHealthy:
			r.healthy++
			if r.healthy >= 3 {
				r.rate = min(maximum, r.rate+max(1, r.rate/4))
				r.healthy = 0
			}
		case reportReduce:
			r.rate = max(minimum, r.rate/2)
			r.healthy = 0
		case reportPause:
			r.rate = minimum
			r.healthy = 0
		}
	}
	return min(maximum, r.rate)
}
