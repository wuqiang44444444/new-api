package service

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shirou/gopsutil/disk"
)

var evidenceDiskPressure atomic.Bool
var evidenceDiskMonitorOnce sync.Once
var evidenceDiskCleanupLast atomic.Int64

// Sampling is independent of the optional performance monitor and collection
// switch: old private evidence must still be reclaimed when capture is disabled.
func startEvidenceDiskMonitor() {
	evidenceDiskMonitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			var lastReport int64
			for {
				now := common.GetTimestamp()
				usage, err := evidenceDiskUsage()
				if err == nil {
					_, err = ObserveEvidenceDiskUsage(now, usage.UsedPercent)
				}
				if err != nil && now-lastReport >= 60 {
					common.SysError("request evidence disk monitoring or cleanup scheduling unavailable")
					lastReport = now
				}
				<-ticker.C
			}
		}()
	})
}
func evidenceDiskUsage() (*disk.UsageStat, error) {
	path, err := filepath.Abs(system_setting.GetTaskRequestEvidenceConfig().StorageDir)
	if err != nil {
		return nil, err
	}
	for {
		if _, err = os.Stat(path); err == nil {
			return disk.Usage(path)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil, err
		}
		path = parent
	}
}

// A high-water observation immediately requests the existing mutually exclusive
// cleanup type with a durable, fixed one-day cutoff. No business storage is used.
func ObserveEvidenceDiskUsage(now int64, usedPercent float64) (bool, error) {
	high := usedPercent > 90
	evidenceDiskPressure.Store(high)
	if !high {
		return false, nil
	}
	last := evidenceDiskCleanupLast.Load()
	if now-last < 60 {
		return false, nil
	}
	active, err := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention)
	if err != nil || active != nil {
		return false, err
	}
	cutoff := now - 86400
	payload := EvidenceCleanupPayload{TargetTimestamp: cutoff, Trigger: "disk_pressure"}
	if _, err = model.CreateEvidenceCleanupSystemTask(cutoff, payload); err != nil {
		if active, lookupErr := model.GetActiveSystemTask(model.SystemTaskTypeTaskRequestEvidenceRetention); lookupErr == nil && active != nil {
			return false, nil
		}
		return false, err
	}
	evidenceDiskCleanupLast.Store(now)
	notifySystemTaskRunner()
	return true, nil
}
