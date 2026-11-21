package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	reportObservationInterval = time.Second
	reportBaselineInterval    = time.Minute
	reportBaselineQuietWindow = 6 * time.Second
	reportBaselineDrainTime   = 2 * time.Second
)

type reportPressure uint8

const (
	reportHealthy reportPressure = iota
	reportReduce
	reportPause
)

type reportAdmissionDecision struct {
	calibrating bool
	pressure    reportPressure
	at          time.Time
}

type reportForegroundWindow struct {
	snapshot    common.ReportForegroundSnapshot
	average     [3]float64
	tail95      [3]time.Duration
	tail99      [3]time.Duration
	requestRate float64
	pressure    reportPressure
	at          time.Time
}

func (w *reportForegroundWindow) observe(at time.Time, next common.ReportForegroundSnapshot, learn bool) reportPressure {
	if !at.After(w.at) {
		return w.pressure
	}
	level := reportHealthy
	if w.at.IsZero() || at.Sub(w.at) > time.Minute {
		w.average = [3]float64{}
		w.tail95 = [3]time.Duration{}
		w.tail99 = [3]time.Duration{}
		level = reportReduce
	} else {
		if next.Active >= 24 && next.Active >= 2*max(1, w.snapshot.Active) {
			level = reportReduce
		}
		for i, counters := range next.Operations {
			operation := common.ReportForegroundOperation(i)
			old := w.snapshot.Operations[i]
			count, failures := counters.Completed-old.Completed, counters.Failed-old.Failed
			slow := counters.Slow - old.Slow
			if i == int(common.ReportForegroundRequest) && count >= 10 && w.requestRate > 0 &&
				float64(count)/at.Sub(w.at).Seconds() > max(w.requestRate*1.5, w.requestRate+25) {
				level = max(level, reportReduce)
			}
			switch {
			case i != int(common.ReportForegroundRequest) && failures > 0:
				level = reportPause
			case count >= 10 && failures >= 3 && failures*100 >= count*5:
				level = reportPause
			case count >= 10 && failures > 0 && failures*100 >= count:
				level = max(level, reportReduce)
			}
			if i != int(common.ReportForegroundRequest) && slow >= 3 && slow*5 >= count {
				level = max(level, reportReduce)
				if slow >= 6 && slow*2 >= count {
					level = reportPause
				}
			}
			minSamples := int64(20)
			if operation == common.ReportForegroundLog {
				minSamples = 50
			}
			if count >= minSamples {
				var measured, above32 int64
				var tail, p99 time.Duration
				for bucket, total := range counters.Latency {
					n := max(0, total-old.Latency[bucket])
					measured += n
					upper := common.ReportForegroundLatencyBucket(operation, bucket)
					if upper > 32*time.Millisecond {
						above32 += n
					}
					if tail == 0 && measured*20 >= count*19 {
						tail = upper
					}
					if count >= 200 && p99 == 0 && measured*100 >= count*99 {
						p99 = upper
					}
				}
				delta := 4 * time.Millisecond
				if operation == common.ReportForegroundRequest {
					delta = 8 * time.Millisecond
				}
				base := w.tail95[i]
				switch {
				case operation == common.ReportForegroundLog && above32*20 >= count,
					base > 0 && tail >= 4*base && tail-base >= 2*delta:
					level = reportPause
				case base > 0 && tail >= 2*base && tail-base >= delta:
					level = max(level, reportReduce)
				}
				if base := w.tail99[i]; base > 0 && p99 >= 2*base && p99-base >= 2*delta {
					level = max(level, reportReduce)
				}
				if learn && level == reportHealthy && tail > 0 {
					w.tail95[i] = tail
					if p99 > 0 {
						w.tail99[i] = p99
					}
				}
			}
			if count >= 10 {
				average := float64(max(0, counters.Milliseconds-old.Milliseconds)) / float64(count)
				if floor := max(1, w.average[i]); w.average[i] > 0 {
					if average >= 2*floor && average-floor >= 20 {
						level = reportPause
					} else if average >= 1.35*floor && average-floor >= 10 {
						level = max(level, reportReduce)
					}
				}
				if learn && level == reportHealthy {
					if w.average[i] == 0 {
						w.average[i] = average
					} else {
						w.average[i] = 0.9*w.average[i] + 0.1*average
					}
				}
			}
		}
	}
	if !w.at.IsZero() && at.Sub(w.at) <= time.Minute {
		count := next.Operations[common.ReportForegroundRequest].Completed - w.snapshot.Operations[common.ReportForegroundRequest].Completed
		w.requestRate = float64(max(0, count)) / at.Sub(w.at).Seconds()
	} else {
		w.requestRate = 0
	}
	w.snapshot, w.at, w.pressure = next, at, level
	return level
}

// One admission state observes the load of both report producers in this
// process. Cross-node foreground snapshots use the existing instance heartbeat.
var sharedReportAdmission = struct {
	sync.Mutex
	db           *gorm.DB
	decision     reportAdmissionDecision
	healthy      int
	last         []model.DailyReportDatabaseLoad
	local        reportForegroundWindow
	peers        map[string]reportForegroundWindow
	quietAt      time.Time
	lastQuiet    time.Time
	peerAt       time.Time
	peerPressure reportPressure
}{}

func checkReportAdmission(ctx context.Context, sample func(context.Context) ([]model.DailyReportDatabaseLoad, error)) reportAdmissionDecision {
	state := &sharedReportAdmission
	state.Lock()
	defer state.Unlock()
	if state.db != model.DB {
		state.db = model.DB
		state.decision = reportAdmissionDecision{}
		state.healthy, state.last, state.local, state.peers = 0, nil, reportForegroundWindow{}, nil
		state.quietAt, state.lastQuiet, state.peerAt, state.peerPressure = time.Time{}, time.Time{}, time.Time{}, reportHealthy
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
	if state.lastQuiet.IsZero() || (elapsed <= 4*reportObservationInterval && now.Sub(state.lastQuiet) >= reportBaselineInterval) {
		state.quietAt, state.lastQuiet = now, now
	}
	quiet := now.Sub(state.quietAt) < reportBaselineQuietWindow
	learning := (quiet && now.Sub(state.quietAt) >= reportBaselineDrainTime) || elapsed > 4*reportObservationInterval && reportArtifactUploads.Load() == 0
	priorForeground := state.local
	currentForeground := common.CurrentReportForeground()
	level := state.local.observe(now, currentForeground, false)
	samples, err := sample(ctx)
	if err != nil || len(samples) == 0 {
		level = reportPause
	}
	for i, current := range samples {
		if current.Probe >= 75*time.Millisecond {
			level = reportPause
		} else if current.Probe >= 35*time.Millisecond {
			level = max(level, reportReduce)
		}
		if i >= len(state.last) || elapsed > 4*reportObservationInterval {
			level = max(level, reportReduce)
			continue
		}
		old := state.last[i]
		waits := current.Pool.WaitCount - old.Pool.WaitCount
		wait := current.Pool.WaitDuration - old.Pool.WaitDuration
		wal := float64(current.WALBytes-old.WALBytes) / elapsed.Seconds()
		switch {
		case waits > 0 && current.Pool.MaxOpenConnections > 0 && current.Pool.InUse >= current.Pool.MaxOpenConnections,
			wait >= 10*time.Millisecond*time.Duration(max(1, waits)), old.WALBytes > 0 && wal >= 24<<20:
			level = reportPause
		case wait >= 3*time.Millisecond*time.Duration(max(1, waits)), old.WALBytes > 0 && wal >= 12<<20:
			level = max(level, reportReduce)
		}
	}
	state.last = samples

	if now.Sub(state.peerAt) >= 5*time.Second {
		state.peerAt, state.peerPressure = now, reportHealthy
		peerCtx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
		instances, err := model.ReportForegroundInstances(peerCtx, now.Unix())
		cancel()
		if err != nil {
			state.peerPressure = reportPause
		}
		peers := make(map[string]reportForegroundWindow, len(instances))
		for _, instance := range instances {
			if instance.NodeName == common.GetNodeIdentity().Name {
				continue
			}
			var info struct {
				ReportForeground *common.ReportForegroundSnapshot `json:"report_foreground"`
			}
			if now.Unix()-instance.LastSeenAt > int64(2*systemInstanceReportInterval/time.Second) ||
				common.UnmarshalJsonStr(instance.Info, &info) != nil || info.ReportForeground == nil {
				state.peerPressure = reportPause
				continue
			}
			peer := state.peers[instance.NodeName]
			if peer.at.Unix() < instance.StartedAt {
				peer = reportForegroundWindow{}
			}
			state.peerPressure = max(state.peerPressure,
				peer.observe(time.Unix(instance.LastSeenAt, 0), *info.ReportForeground, learning))
			peers[instance.NodeName] = peer
		}
		for node, peer := range state.peers {
			if _, seen := peers[node]; !seen && now.Sub(peer.at) <= 2*time.Duration(model.SystemInstanceStaleAfterSeconds)*time.Second {
				state.peerPressure = max(state.peerPressure, reportReduce)
				peers[node] = peer
			}
		}
		state.peers = peers
	}
	level = max(level, state.peerPressure)
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
	if level < reportPause && previous.pressure == reportPause && state.healthy < 3 {
		calibrating = previous.calibrating && level == reportHealthy
		level = reportPause
	}
	state.decision = reportAdmissionDecision{pressure: level, at: now, calibrating: calibrating}
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
