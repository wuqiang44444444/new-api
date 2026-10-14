package service

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shirou/gopsutil/disk"
)

// CheckEvidenceCleanupHealth reads durable scheduling/backlog facts and the
// filesystem containing the evidence root without creating a directory.
func CheckEvidenceCleanupHealth(now int64) (*model.EvidenceCleanupHealth, error) {
	health, err := model.GetEvidenceCleanupHealth(now, EvidenceRetentionSeconds())
	if err != nil {
		return nil, err
	}
	path, err := filepath.Abs(system_setting.GetTaskRequestEvidenceConfig().StorageDir)
	if err != nil {
		return nil, err
	}
	for {
		_, err = os.Stat(path)
		if err == nil {
			break
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
	usage, err := disk.Usage(path)
	if err != nil {
		return nil, err
	}
	health.FreeBytes = &usage.Free
	if usage.InodesTotal > 0 {
		health.FreeInodes = &usage.InodesFree
	}
	health.Alerts = evidenceCleanupAlerts(health, usage.UsedPercent, usage.InodesUsedPercent, float64(common.GetPerformanceMonitorConfig().DiskThreshold))
	return health, nil
}

func evidenceCleanupAlerts(health *model.EvidenceCleanupHealth, diskUsed, inodeUsed, diskThreshold float64) []string {
	alerts := []string{}
	if health.ConsecutiveFailures >= 2 {
		alerts = append(alerts, "consecutive_failures")
	}
	lastSuccess := health.LastSucceededAt
	if lastSuccess == 0 {
		lastSuccess = health.MonitoringSince
	}
	if health.CheckedAt-lastSuccess >= 2*3600 {
		alerts = append(alerts, "cleanup_stalled")
	}
	if (health.PendingIndexes > 0 || health.PendingDatabaseIndexes > 0) && len(alerts) > 0 {
		alerts = append(alerts, "cleanup_backlog")
	}
	if diskThreshold <= 0 || diskThreshold > 100 {
		diskThreshold = 90
	}
	if diskUsed >= diskThreshold {
		alerts = append(alerts, "disk_space_low")
	}
	if health.FreeInodes != nil && inodeUsed >= 95 {
		alerts = append(alerts, "inodes_low")
	}
	return alerts
}
