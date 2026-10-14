package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMidjourneyEvidenceAnchorContract(t *testing.T) {
	setupEvidenceLifecycleDatabase(t)
	runMidjourneyEvidenceAnchorContract(t)
}

// Shared with real MySQL/PostgreSQL tests. MJ billing fields and mutable
// SubmitTime cannot supply or replace the evidence creation anchor.
func runMidjourneyEvidenceAnchorContract(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Midjourney{}))
	now := common.GetTimestamp()
	task := &Midjourney{UserId: 7, MjId: "mj-anchor-contract", SubmitTime: now * 1000}
	require.NoError(t, DB.Create(task).Error)
	seed := TaskRequestEvidence{UserID: 7, AppID: 3, RequestID: "mj-request-contract"}
	_, err := GetMidjourneyMediaEvidence(seed, task.MjId, now, 7*86400)
	assert.ErrorIs(t, err, ErrTaskRequestEvidenceCreationTime)
	seed.CreatedAt = now - 3600
	index, err := GetMidjourneyMediaEvidence(seed, task.MjId, now, 7*86400)
	require.NoError(t, err)
	assert.Equal(t, seed.RequestID, index.RequestID)
	assert.Equal(t, seed.CreatedAt+7*86400, index.RetainUntil)
	seed.CreatedAt = 0
	require.NoError(t, DB.Model(task).UpdateColumn("submit_time", (now+86400)*1000).Error)
	queried, err := GetMidjourneyMediaEvidence(seed, task.MjId, now, 7*86400)
	require.NoError(t, err)
	assert.Equal(t, index.Id, queried.Id)
	assert.Equal(t, index.CreatedAt, queried.CreatedAt)
	assert.Equal(t, index.RetainUntil, queried.RetainUntil)
	_, err = GetMidjourneyMediaEvidence(seed, task.MjId, index.CreatedAt+7*86400, 7*86400)
	require.NoError(t, err, "exact expiration remains within the existing boundary")
	_, err = GetMidjourneyMediaEvidence(seed, task.MjId, index.CreatedAt+7*86400+1, 7*86400)
	assert.ErrorIs(t, err, ErrTaskRequestEvidenceBodyClosed)
	require.NoError(t, RequestTaskRequestEvidenceCleanup(index.Id, now))
	_, err = GetMidjourneyMediaEvidence(seed, task.MjId, now, 7*86400)
	assert.ErrorIs(t, err, ErrTaskRequestEvidenceBodyClosed)
	require.NoError(t, DB.Delete(index).Error)
	_, err = GetMidjourneyMediaEvidence(seed, task.MjId, now, 7*86400)
	assert.ErrorIs(t, err, ErrTaskRequestEvidenceCreationTime)
	var count int64
	require.NoError(t, DB.Model(&TaskRequestEvidence{}).Where("task_id = ?", task.MjId).Count(&count).Error)
	assert.Zero(t, count)
}
