package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceRetentionDeadline(t *testing.T) {
	day := int64(86400)
	created := int64(1000000)
	// Zero retention_until is bounded by the current retention window.
	assert.Equal(t, created+2*day, evidenceRetentionDeadline(created, 0, 2))
	// An earlier non-zero retain_until wins.
	assert.Equal(t, created+day, evidenceRetentionDeadline(created, created+day, 2))
	// Current window shorter than a historical longer period wins.
	assert.Equal(t, created+2*day, evidenceRetentionDeadline(created, created+30*day, 2))
	// An out-of-range window (bogus disabled-state config) is clamped to 2 days.
	assert.Equal(t, created+2*day, evidenceRetentionDeadline(created, 0, 99))
}

func TestEvidenceBodyExpiredBoundaries(t *testing.T) {
	day := int64(86400)
	created := int64(1000000)
	// Exactly at the deadline the body is still readable; expired after it.
	require.False(t, evidenceBodyExpired(&model.TaskRequestEvidence{CreatedAt: created, RetainUntil: 0}, created+2*day))
	require.True(t, evidenceBodyExpired(&model.TaskRequestEvidence{CreatedAt: created, RetainUntil: 0}, created+2*day+1))
	require.True(t, evidenceBodyExpired(&model.TaskRequestEvidence{CreatedAt: created, RetainUntil: created + day}, created+day+1))
	// A persisted cleanup request closes reads immediately.
	require.True(t, evidenceBodyExpired(&model.TaskRequestEvidence{CreatedAt: created, CleanupRequestedAt: created}, created+1))
	require.True(t, evidenceBodyExpired(nil, created))
}
