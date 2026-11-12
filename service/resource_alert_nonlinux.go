//go:build windows

package service

import (
	"os"
	"path/filepath"
)

func readCgroupResourceUsage() (resourceCPUCounters, float64, bool) {
	return resourceCPUCounters{}, 0, false
}

func resourceDiskID(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return ""
	}
	return filepath.VolumeName(resolved) + ":" + info.Name()
}
