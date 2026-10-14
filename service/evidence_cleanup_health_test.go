package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestEvidenceCleanupHealthAlertsProtectRecoveryAndStorageThresholds(t *testing.T) {
	freeInodes := uint64(0)
	for _, tc := range []struct {
		name        string
		health      model.EvidenceCleanupHealth
		disk, inode float64
		want        []string
	}{
		{"startup grace", model.EvidenceCleanupHealth{CheckedAt: 10000, MonitoringSince: 10000, PendingIndexes: 3}, 10, 10, []string{}},
		{"two failed runs with backlog", model.EvidenceCleanupHealth{CheckedAt: 10000, LastSucceededAt: 9000, ConsecutiveFailures: 2, PendingIndexes: 3}, 10, 10, []string{"consecutive_failures", "cleanup_backlog"}},
		{"two periods without success", model.EvidenceCleanupHealth{CheckedAt: 10000, LastSucceededAt: 2800, PendingIndexes: 1}, 10, 10, []string{"cleanup_stalled", "cleanup_backlog"}},
		{"disk threshold", model.EvidenceCleanupHealth{CheckedAt: 10000, LastSucceededAt: 10000}, 90, 0, []string{"disk_space_low"}},
		{"inode exhaustion", model.EvidenceCleanupHealth{CheckedAt: 10000, LastSucceededAt: 10000, FreeInodes: &freeInodes}, 10, 100, []string{"inodes_low"}},
		{"inode metrics unavailable", model.EvidenceCleanupHealth{CheckedAt: 10000, LastSucceededAt: 10000}, 10, 100, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, evidenceCleanupAlerts(&tc.health, tc.disk, tc.inode, 90)) })
	}
}
