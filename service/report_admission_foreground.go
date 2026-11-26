package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

type reportForegroundWindow struct {
	snapshot    common.ReportForegroundSnapshot
	average     [3]float64
	tail95      [3]time.Duration
	tail99      [3]time.Duration
	requestRate float64
	pressure    reportPressureObservation
	at          time.Time
}

func (w *reportForegroundWindow) observe(at time.Time, next common.ReportForegroundSnapshot, learn bool) reportPressure {
	if !at.After(w.at) {
		return w.pressure.level
	}
	var pressure reportPressureObservation
	const (
		maxGap               = time.Minute
		minRequestSamples    = 10
		foregroundFailure    = 1
		requestPauseFailures = 3
		requestPauseRate     = 5
		requestReduceRate    = 1
		slowPauseCount       = 6
		slowPauseDivisor     = 2
		slowReduceCount      = 3
		slowReduceDivisor    = 5
	)
	if w.at.IsZero() || at.Sub(w.at) > maxGap {
		w.average = [3]float64{}
		w.tail95 = [3]time.Duration{}
		w.tail99 = [3]time.Duration{}
		if w.at.IsZero() {
			pressure.record(reportReduce, "trigger=foreground_baseline sample=missing threshold=previous_sample")
		} else {
			pressure.record(reportReduce, "trigger=foreground_sample_gap sample=%s threshold=%s", at.Sub(w.at), maxGap)
		}
	} else {
		if limit := max(24, 2*max(1, w.snapshot.Active)); next.Active >= limit {
			pressure.record(reportReduce, "trigger=foreground_active sample=%d threshold=%d previous=%d", next.Active, limit, w.snapshot.Active)
		}
		for i, counters := range next.Operations {
			operation := common.ReportForegroundOperation(i)
			old := w.snapshot.Operations[i]
			count, failures := counters.Completed-old.Completed, counters.Failed-old.Failed
			slow := counters.Slow - old.Slow
			if operation == common.ReportForegroundRequest && count >= minRequestSamples && w.requestRate > 0 {
				rate, limit := float64(count)/at.Sub(w.at).Seconds(), max(w.requestRate*1.5, w.requestRate+25)
				if rate > limit {
					pressure.record(reportReduce, "trigger=request_rate sample=%.2f/s threshold=%.2f/s completed=%d", rate, limit, count)
				}
			}
			operationName := [...]string{
				common.ReportForegroundRequest:    "request",
				common.ReportForegroundSettlement: "settlement",
				common.ReportForegroundLog:        "log_write",
			}[operation]
			switch {
			case operation != common.ReportForegroundRequest && failures >= foregroundFailure:
				pressure.record(reportPause, "trigger=foreground_failure operation=%s sample=%d threshold=%d completed=%d",
					operationName, failures, foregroundFailure, count)
			case count >= minRequestSamples && failures >= requestPauseFailures && failures*100 >= count*requestPauseRate:
				pressure.record(reportPause, "trigger=request_failure_rate operation=%s failures=%d completed=%d threshold_failures=%d threshold_completed=%d threshold_rate=%d%%",
					operationName, failures, count, requestPauseFailures, minRequestSamples, requestPauseRate)
			case count >= minRequestSamples && failures >= foregroundFailure && failures*100 >= count*requestReduceRate:
				pressure.record(reportReduce, "trigger=request_failure_rate operation=%s failures=%d completed=%d threshold_failures=%d threshold_completed=%d threshold_rate=%d%%",
					operationName, failures, count, foregroundFailure, minRequestSamples, requestReduceRate)
			}
			if operation != common.ReportForegroundRequest && slow >= slowReduceCount && slow*slowReduceDivisor >= count {
				if slow >= slowPauseCount && slow*slowPauseDivisor >= count {
					pressure.record(reportPause, "trigger=foreground_slow_rate operation=%s slow=%d completed=%d threshold_slow=%d threshold_rate=%d%%",
						operationName, slow, count, slowPauseCount, 100/slowPauseDivisor)
				} else {
					pressure.record(reportReduce, "trigger=foreground_slow_rate operation=%s slow=%d completed=%d threshold_slow=%d threshold_rate=%d%%",
						operationName, slow, count, slowReduceCount, 100/slowReduceDivisor)
				}
			}
			w.observeLatency(&pressure, operation, operationName, counters, old, count, learn)
		}
	}
	if !w.at.IsZero() && at.Sub(w.at) <= maxGap {
		count := next.Operations[common.ReportForegroundRequest].Completed - w.snapshot.Operations[common.ReportForegroundRequest].Completed
		w.requestRate = float64(max(0, count)) / at.Sub(w.at).Seconds()
	} else {
		w.requestRate = 0
	}
	w.snapshot, w.at, w.pressure = next, at, pressure
	return pressure.level
}

func (w *reportForegroundWindow) observeLatency(pressure *reportPressureObservation, operation common.ReportForegroundOperation, operationName string, counters, old common.ReportForegroundCounters, count int64, learn bool) {
	const (
		logTailLimit   = 32 * time.Millisecond
		logTailDivisor = 20
	)
	i := int(operation)
	minSamples := int64(20)
	if operation == common.ReportForegroundLog {
		minSamples = 50
	}
	if count >= minSamples {
		var measured, aboveLogTailLimit int64
		var tail, p99 time.Duration
		for bucket, total := range counters.Latency {
			n := max(0, total-old.Latency[bucket])
			measured += n
			upper := common.ReportForegroundLatencyBucket(operation, bucket)
			if upper > logTailLimit {
				aboveLogTailLimit += n
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
		pauseLimit, reduceLimit := max(4*base, base+2*delta), max(2*base, base+delta)
		if operation == common.ReportForegroundLog && aboveLogTailLimit*logTailDivisor >= count {
			pressure.record(reportPause, "trigger=log_write_tail above_limit=%d latency_limit=%s completed=%d threshold_rate=%d%% threshold_completed=%d",
				aboveLogTailLimit, logTailLimit, count, 100/logTailDivisor, minSamples)
		}
		switch {
		case base > 0 && tail >= pauseLimit:
			pressure.record(reportPause, "trigger=foreground_p95 operation=%s sample=%s baseline=%s threshold=%s completed=%d", operationName, tail, base, pauseLimit, count)
		case base > 0 && tail >= reduceLimit:
			pressure.record(reportReduce, "trigger=foreground_p95 operation=%s sample=%s baseline=%s threshold=%s completed=%d", operationName, tail, base, reduceLimit, count)
		}
		base99 := w.tail99[i]
		if limit := max(2*base99, base99+2*delta); base99 > 0 && p99 >= limit {
			pressure.record(reportReduce, "trigger=foreground_p99 operation=%s sample=%s baseline=%s threshold=%s completed=%d", operationName, p99, base99, limit, count)
		}
		if learn && pressure.level == reportHealthy && tail > 0 {
			w.tail95[i] = tail
			if p99 > 0 {
				w.tail99[i] = p99
			}
		}
	}
	if count >= 10 {
		average := float64(max(0, counters.Milliseconds-old.Milliseconds)) / float64(count)
		if floor := max(1, w.average[i]); w.average[i] > 0 {
			const (
				pauseFactor   = 2
				pauseDeltaMS  = 20
				reduceFactor  = 1.35
				reduceDeltaMS = 10
			)
			if average >= pauseFactor*floor && average-floor >= pauseDeltaMS {
				pressure.record(reportPause, "trigger=foreground_average_latency operation=%s sample=%.2fms baseline=%.2fms threshold=%.2fms completed=%d", operationName, average, floor, max(pauseFactor*floor, floor+pauseDeltaMS), count)
			} else if average >= reduceFactor*floor && average-floor >= reduceDeltaMS {
				pressure.record(reportReduce, "trigger=foreground_average_latency operation=%s sample=%.2fms baseline=%.2fms threshold=%.2fms completed=%d", operationName, average, floor, max(reduceFactor*floor, floor+reduceDeltaMS), count)
			}
		}
		if learn && pressure.level == reportHealthy {
			if w.average[i] == 0 {
				w.average[i] = average
			} else {
				w.average[i] = 0.9*w.average[i] + 0.1*average
			}
		}
	}
}
