//go:build linux

package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func readCgroupResourceUsage() (resourceCPUCounters, float64, bool) {
	groups, _ := os.ReadFile("/proc/self/cgroup")
	mounts, _ := os.ReadFile("/proc/self/mountinfo")
	v2 := resolveResourceCgroup(string(groups), string(mounts), "")
	if v2.path != "" {
		var usage float64
		for _, line := range strings.Split(cgroupText(v2.path, "cpu.stat"), "\n") {
			if value, ok := strings.CutPrefix(line, "usage_usec "); ok {
				n, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
				usage = n / 1e6
			}
		}
		counters := resourceCPUCounters{used: usage, cores: float64(runtime.NumCPU())}
		for dir := v2.path; ; dir = filepath.Dir(dir) {
			fields := strings.Fields(cgroupText(dir, "cpu.max"))
			if len(fields) == 2 {
				counters.cores = limitedCPUCores(counters.cores, fields[0], fields[1])
			}
			if dir == v2.mount || dir == "/" {
				break
			}
		}
		memory, ok := cgroupMemory(v2, "memory.current", "memory.max")
		if usage == 0 && cgroupText(v2.path, "cpu.stat") == "" {
			counters = resourceCPUCounters{}
		}
		return counters, memory, ok
	}
	cpu := resolveResourceCgroup(string(groups), string(mounts), "cpu")
	accounting := resolveResourceCgroup(string(groups), string(mounts), "cpuacct")
	var counters resourceCPUCounters
	if raw := cgroupText(accounting.path, "cpuacct.usage"); raw != "" {
		n, _ := strconv.ParseFloat(raw, 64)
		counters = resourceCPUCounters{used: n / 1e9, cores: float64(runtime.NumCPU())}
		for dir := cpu.path; dir != ""; dir = filepath.Dir(dir) {
			counters.cores = limitedCPUCores(counters.cores, cgroupText(dir, "cpu.cfs_quota_us"), cgroupText(dir, "cpu.cfs_period_us"))
			if dir == cpu.mount || dir == "/" {
				break
			}
		}
	}
	memory, ok := cgroupMemory(resolveResourceCgroup(string(groups), string(mounts), "memory"), "memory.usage_in_bytes", "memory.limit_in_bytes")
	return counters, memory, ok
}

type resourceCgroup struct{ path, mount string }

func resolveResourceCgroup(groups, mounts, controller string) resourceCgroup {
	var group string
	for _, line := range strings.Split(groups, "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && (parts[1] == controller || controller != "" && strings.Contains(","+parts[1]+",", ","+controller+",")) {
			group = parts[2]
			break
		}
	}
	if group == "" {
		return resourceCgroup{}
	}
	for _, line := range strings.Split(mounts, "\n") {
		left, right, ok := strings.Cut(line, " - ")
		mount, fs := strings.Fields(left), strings.Fields(right)
		if !ok || len(mount) < 5 || len(fs) < 3 {
			continue
		}
		if controller == "" && fs[0] != "cgroup2" || controller != "" && (fs[0] != "cgroup" || !strings.Contains(","+fs[2]+",", ","+controller+",")) {
			continue
		}
		root := mount[3]
		relative, err := filepath.Rel(root, group)
		if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
			if group != "/" {
				continue
			}
			relative = "."
		}
		return resourceCgroup{filepath.Join(mount[4], relative), mount[4]}
	}
	return resourceCgroup{}
}

func limitedCPUCores(current float64, quota, period string) float64 {
	q, e1 := strconv.ParseFloat(quota, 64)
	p, e2 := strconv.ParseFloat(period, 64)
	if e1 == nil && e2 == nil && q > 0 && p > 0 {
		return min(current, q/p)
	}
	return current
}

func cgroupText(dir, name string) string {
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cgroupMemory(group resourceCgroup, usageFile, limitFile string) (float64, bool) {
	var percent float64
	var valid bool
	for dir := group.path; dir != ""; dir = filepath.Dir(dir) {
		usage, e1 := strconv.ParseUint(cgroupText(dir, usageFile), 10, 64)
		limit, e2 := strconv.ParseUint(cgroupText(dir, limitFile), 10, 64)
		if e1 == nil && e2 == nil && limit > 0 && limit < 1<<60 {
			percent = max(percent, 100*float64(usage)/float64(limit))
			valid = true
		}
		if dir == group.mount || dir == "/" {
			break
		}
	}
	return min(100, percent), valid
}

func resourceDiskID(path string) string {
	var stat syscall.Stat_t
	if syscall.Stat(path, &stat) != nil {
		return ""
	}
	return strconv.FormatUint(uint64(stat.Dev), 10)
}
