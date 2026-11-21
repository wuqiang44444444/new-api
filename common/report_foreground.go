package common

import (
	"sync/atomic"
	"time"
)

type ReportForegroundOperation uint8

const (
	ReportForegroundRequest ReportForegroundOperation = iota
	ReportForegroundSettlement
	ReportForegroundLog
	reportForegroundOperations
)

// Each node publishes its cumulative totals with its existing instance heartbeat.
type ReportForegroundCounters struct {
	Completed    int64    `json:"completed"`
	Failed       int64    `json:"failed"`
	Milliseconds int64    `json:"milliseconds"`
	Slow         int64    `json:"slow"`
	Latency      [8]int64 `json:"latency"`
}

type ReportForegroundSnapshot struct {
	Active     int64                                                `json:"active"`
	Operations [reportForegroundOperations]ReportForegroundCounters `json:"operations"`
}

var reportForegroundActive atomic.Int64
var reportForegroundTotals [reportForegroundOperations]struct {
	completed, failed, milliseconds, slow atomic.Int64
	latency                               [8]atomic.Int64
}

var reportForegroundLatencyBounds = [reportForegroundOperations][8]int64{
	ReportForegroundRequest:    {4, 8, 16, 32, 64, 128, 512, 4096},
	ReportForegroundSettlement: {1, 2, 4, 8, 16, 32, 64, 128},
	ReportForegroundLog:        {1, 2, 4, 8, 16, 32, 64, 128},
}

func ReportForegroundLatencyBucket(operation ReportForegroundOperation, index int) time.Duration {
	return time.Duration(reportForegroundLatencyBounds[operation][index]) * time.Millisecond
}

type ReportForegroundSample struct {
	operation ReportForegroundOperation
	start     time.Time
}

func BeginReportForeground(operation ReportForegroundOperation) ReportForegroundSample {
	if operation == ReportForegroundRequest {
		reportForegroundActive.Add(1)
	}
	return ReportForegroundSample{operation: operation, start: time.Now()}
}

func (s ReportForegroundSample) Finish(failed bool) {
	elapsed := time.Since(s.start)
	counters := &reportForegroundTotals[s.operation]
	ms := elapsed.Milliseconds()
	counters.milliseconds.Add(ms)
	bounds := &reportForegroundLatencyBounds[s.operation]
	bucket := 0
	for bucket < len(bounds)-1 && elapsed > time.Duration(bounds[bucket])*time.Millisecond {
		bucket++
	}
	counters.latency[bucket].Add(1)
	if failed {
		counters.failed.Add(1)
	}
	if (s.operation == ReportForegroundRequest && elapsed >= 5*time.Second) ||
		(s.operation != ReportForegroundRequest && elapsed >= 100*time.Millisecond) {
		counters.slow.Add(1)
	}
	counters.completed.Add(1)
	if s.operation == ReportForegroundRequest {
		reportForegroundActive.Add(-1)
	}
}

func CurrentReportForeground() ReportForegroundSnapshot {
	snapshot := ReportForegroundSnapshot{Active: reportForegroundActive.Load()}
	for i := range snapshot.Operations {
		counters := &reportForegroundTotals[i]
		snapshot.Operations[i] = ReportForegroundCounters{
			Completed: counters.completed.Load(), Failed: counters.failed.Load(),
			Milliseconds: counters.milliseconds.Load(), Slow: counters.slow.Load(),
		}
		for j := range counters.latency {
			snapshot.Operations[i].Latency[j] = counters.latency[j].Load()
		}
	}
	return snapshot
}
