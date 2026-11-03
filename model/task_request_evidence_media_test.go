package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestIndependentEvidenceWindowContract(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	runIndependentEvidenceWindowContract(t)
}

// Shared with the existing opt-in real database tests. Task age and a closed
// earlier request cannot prevent a new actual request from acquiring a window.
func runIndependentEvidenceWindowContract(t *testing.T) {
	t.Helper()
	now := common.GetTimestamp()
	first := &TaskRequestEvidence{TaskID: "same-task", UserID: 7, AppID: 3, RequestID: "old-request", CreatedAt: now - 3*86400, RetainUntil: now - 86400}
	require.NoError(t, CreateTaskRequestEvidence(first))
	require.NoError(t, RequestTaskRequestEvidenceCleanup(first.Id, now))
	second := &TaskRequestEvidence{TaskID: first.TaskID, UserID: 7, AppID: 3, RequestID: "new-request", CreatedAt: now, RetainUntil: now + 2*86400}
	require.NoError(t, CreateTaskRequestEvidence(second))
	require.NoError(t, CreateTaskRequestEvidenceEvent(&TaskRequestEvidenceEvent{EvidenceId: second.Id}, 2*86400))
	assert.ErrorIs(t, CreateTaskRequestEvidenceEvent(&TaskRequestEvidenceEvent{EvidenceId: first.Id}, 2*86400), ErrTaskRequestEvidenceBodyClosed)
	assert.NotEqual(t, first.Id, second.Id)
	assert.Equal(t, now+2*86400, second.RetainUntil)
}
