package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func observeReportDatabasePressure(pressure *reportPressureObservation, samples, previous []model.DailyReportDatabaseLoad, elapsed time.Duration) {
	const (
		probePause  = 100 * time.Millisecond
		probeReduce = 55 * time.Millisecond
		maxDBGap    = 4 * reportObservationInterval
		walPause    = 24 << 20
		walReduce   = 12 << 20
	)
	for i, current := range samples {
		source := model.DailyReportMainDatabaseSource
		if i > 0 {
			source = model.DailyReportLogDatabaseSource
		}
		if current.Probe >= probePause {
			pressure.record(reportPause, "source=%s trigger=db_probe_latency sample=%s threshold=%s", source, current.Probe, probePause)
		} else if current.Probe >= probeReduce {
			pressure.record(reportReduce, "source=%s trigger=db_probe_latency sample=%s threshold=%s", source, current.Probe, probeReduce)
		}
		if i >= len(previous) {
			pressure.record(reportReduce, "source=%s trigger=db_baseline sample=missing threshold=previous_sample", source)
			continue
		}
		if elapsed > maxDBGap {
			pressure.record(reportReduce, "source=%s trigger=db_sample_gap sample=%s threshold=%s", source, elapsed, maxDBGap)
			continue
		}
		old := previous[i]
		waits := current.Pool.WaitCount - old.Pool.WaitCount
		wait := current.Pool.WaitDuration - old.Pool.WaitDuration
		wal := float64(current.WALBytes-old.WALBytes) / elapsed.Seconds()
		if waits > 0 && current.Pool.MaxOpenConnections > 0 && current.Pool.InUse >= current.Pool.MaxOpenConnections {
			pressure.record(reportPause, "source=%s trigger=db_pool_full sample=%d threshold=%d new_waits=%d", source, current.Pool.InUse, current.Pool.MaxOpenConnections, waits)
		}
		waitPause := 10 * time.Millisecond * time.Duration(max(1, waits))
		waitReduce := 3 * time.Millisecond * time.Duration(max(1, waits))
		switch {
		case wait >= waitPause:
			pressure.record(reportPause, "source=%s trigger=db_pool_wait sample=%s threshold=%s new_waits=%d", source, wait, waitPause, waits)
		case wait >= waitReduce:
			pressure.record(reportReduce, "source=%s trigger=db_pool_wait sample=%s threshold=%s new_waits=%d", source, wait, waitReduce, waits)
		}
		switch {
		case old.WALBytes > 0 && wal >= walPause:
			pressure.record(reportPause, "source=%s trigger=db_wal_rate sample=%.2fB/s threshold=%dB/s", source, wal, walPause)
		case old.WALBytes > 0 && wal >= walReduce:
			pressure.record(reportReduce, "source=%s trigger=db_wal_rate sample=%.2fB/s threshold=%dB/s", source, wal, walReduce)
		}
	}
}

func observeReportPeerPressure(now time.Time, instances []model.SystemInstance, err error, timeout time.Duration, previous map[string]reportForegroundWindow, learn bool) (map[string]reportForegroundWindow, reportPressureObservation) {
	var pressure reportPressureObservation
	if err != nil {
		pressure.record(reportPause, "source=peer_foreground trigger=peer_query_error sample=%s threshold=successful_query timeout=%s", reportSampleErrorStatus(err), timeout)
	}
	peers := make(map[string]reportForegroundWindow, len(instances))
	for _, instance := range instances {
		if instance.NodeName == common.GetNodeIdentity().Name {
			continue
		}
		var info struct {
			ReportForeground *common.ReportForegroundSnapshot `json:"report_foreground"`
		}
		const maxPeerAge = int64(2 * systemInstanceReportInterval / time.Second)
		if age := now.Unix() - instance.LastSeenAt; age > maxPeerAge {
			pressure.record(reportPause, "source=peer_foreground trigger=peer_heartbeat_age sample=%ds threshold=%ds", age, maxPeerAge)
			continue
		}
		if common.UnmarshalJsonStr(instance.Info, &info) != nil {
			pressure.record(reportPause, "source=peer_foreground trigger=peer_sample_unavailable sample=invalid threshold=valid_sample")
			continue
		}
		if info.ReportForeground == nil {
			pressure.record(reportPause, "source=peer_foreground trigger=peer_sample_unavailable sample=missing threshold=valid_sample")
			continue
		}
		peer := previous[instance.NodeName]
		if peer.at.Unix() < instance.StartedAt {
			peer = reportForegroundWindow{}
		}
		peer.observe(time.Unix(instance.LastSeenAt, 0), *info.ReportForeground, learn)
		pressure.merge("source=peer_foreground ", peer.pressure)
		peers[instance.NodeName] = peer
	}
	for node, peer := range previous {
		const peerRetention = 2 * time.Duration(model.SystemInstanceStaleAfterSeconds) * time.Second
		if _, seen := peers[node]; !seen && now.Sub(peer.at) <= peerRetention {
			pressure.record(reportReduce, "source=peer_foreground trigger=peer_missing sample=missing threshold=present peer_age=%s retention=%s", now.Sub(peer.at), peerRetention)
			peers[node] = peer
		}
	}
	return peers, pressure
}
