//go:build linux

package service

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResourceCgroupUsesNamespaceMountAndQuota(t *testing.T) {
	mounts := "31 24 0:25 /container/fixture /sys/fs/cgroup rw - cgroup2 cgroup rw"
	assert.Equal(t, resourceCgroup{"/sys/fs/cgroup", "/sys/fs/cgroup"}, resolveResourceCgroup("0::/\n", mounts, ""))
	assert.Equal(t, resourceCgroup{"/sys/fs/cgroup/subgroup", "/sys/fs/cgroup"}, resolveResourceCgroup("0::/container/fixture/subgroup\n", mounts, ""))
	assert.Empty(t, resolveResourceCgroup("0::/outside\n", mounts, ""))
	assert.Equal(t, 0.5, limitedCPUCores(4, "50000", "100000"))
	assert.Equal(t, 0.5, limitedCPUCores(0.5, "max", "100000"))
}

func TestResourceSamplerInLimitedContainer(t *testing.T) {
	if os.Getenv("TEST_MAIL_RESOURCE_CGROUP") != "1" {
		t.Skip("requires local container with half a CPU and a 256 MiB memory limit")
	}
	cpu, memory, memoryOK := readCgroupResourceUsage()
	require.True(t, memoryOK)
	assert.InDelta(t, 0.5, cpu.cores, 0.001)
	assert.Greater(t, cpu.used, 0.0)
	assert.Greater(t, memory, 0.0)
	assert.LessOrEqual(t, memory, 100.0)
}
