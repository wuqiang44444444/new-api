//go:build !linux && !windows

package service

import (
	"strconv"

	"golang.org/x/sys/unix"
)

func readCgroupResourceUsage() (resourceCPUCounters, float64, bool) {
	return resourceCPUCounters{}, 0, false
}

func resourceDiskID(path string) string {
	var stat unix.Stat_t
	if unix.Stat(path, &stat) != nil {
		return ""
	}
	return strconv.FormatUint(uint64(stat.Dev), 10)
}
