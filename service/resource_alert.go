package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

const resourceSampleInterval = 15 * time.Second

type resourceCPUCounters struct {
	used  float64
	total float64
	cores float64
}

func (c resourceCPUCounters) percent(previous resourceCPUCounters, elapsed time.Duration) (float64, bool) {
	if c.cores != previous.cores || c.used < previous.used || elapsed <= 0 {
		return 0, false
	}
	total := c.total - previous.total
	if c.cores > 0 {
		total = c.cores * elapsed.Seconds()
	}
	if total <= 0 {
		return 0, false
	}
	return min(100, 100*(c.used-previous.used)/total), true
}

type resourceObservation struct {
	key       string
	label     string
	usage     float64
	threshold int
}

var resourceAlertMonitorOnce sync.Once

func startResourceAlertMonitor() {
	resourceAlertMonitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(resourceSampleInterval)
			defer ticker.Stop()
			node := common.NodeName
			known := make(map[string]bool)
			var previous resourceCPUCounters
			var previousAt time.Time
			for now := range ticker.C {
				if len(operation_setting.ParseErrorReportRecipients(operation_setting.ErrorReportSettingSnapshot().Recipients)) == 0 {
					continue
				}
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				thresholds := operation_setting.GetResourceAlertThresholds()
				counters, memory, memoryOK := readResourceUsage(ctx)
				var observations []resourceObservation
				if usage, ok := counters.percent(previous, now.Sub(previousAt)); ok && !previousAt.IsZero() {
					observations = append(observations, resourceObservation{"cpu", "CPU", usage, thresholds.CPU})
				}
				previous, previousAt = counters, now
				if memoryOK {
					observations = append(observations, resourceObservation{"memory", "内存 / Memory", memory, thresholds.Memory})
				}
				observations = append(observations, resourceDisks(ctx, thresholds.Disk)...)
				for _, observation := range observations {
					high := observation.usage >= float64(observation.threshold)
					if last, ok := known[observation.key]; ok && last == high {
						continue
					}
					if recorded, err := observeResourceAlert(ctx, node, observation, now.Unix()); err == nil {
						if !recorded {
							continue
						}
						known[observation.key] = high
					} else {
						common.SysError("resource alert state could not be recorded")
					}
				}
				cancel()
				queueResourceAlertDeliveries()
			}
		}()
	})
}

func readResourceUsage(ctx context.Context) (resourceCPUCounters, float64, bool) {
	counters, usage, hasMemory := readCgroupResourceUsage()
	if counters.cores == 0 {
		if times, err := cpu.TimesWithContext(ctx, false); err == nil && len(times) == 1 {
			t := times[0]
			total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
			counters = resourceCPUCounters{used: total - t.Idle - t.Iowait, total: total}
		}
	}
	if !hasMemory {
		if memory, err := mem.VirtualMemoryWithContext(ctx); err == nil && memory.Total > 0 {
			usage, hasMemory = memory.UsedPercent, true
		}
	}
	return counters, usage, hasMemory
}

func resourceDisks(ctx context.Context, threshold int) []resourceObservation {
	dataDir := "."
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) && !strings.Contains(common.SQLitePath, "mode=memory") && common.SQLitePath != ":memory:" {
		name, _, _ := strings.Cut(strings.TrimPrefix(common.SQLitePath, "file:"), "?")
		dataDir = filepath.Dir(name)
	}
	paths := []struct{ key, label, path string }{
		{"disk_data", "数据卷 / Data volume", dataDir},
		{"disk_log", "日志卷 / Log volume", *common.LogDir},
	}
	var observations []resourceObservation
	seen := map[string]bool{}
	for _, p := range paths {
		if p.path == "" {
			continue
		}
		if usage, err := disk.UsageWithContext(ctx, p.path); err == nil && usage.Total > 0 {
			id := resourceDiskID(p.path)
			if id != "" && seen[id] {
				continue
			}
			seen[id] = true
			observations = append(observations, resourceObservation{p.key, p.label, usage.UsedPercent, threshold})
		}
	}
	return observations
}

func observeResourceAlert(ctx context.Context, node string, observation resourceObservation, at int64) (bool, error) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(node+"\x00"+observation.key)))
	if observation.usage < float64(observation.threshold) {
		return true, model.CloseResourceAlert(ctx, key, at)
	}
	return recordResourceAlert(ctx, key, node, observation, at)
}
