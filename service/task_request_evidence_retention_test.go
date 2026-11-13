package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceWindowFreezesDatabaseRetentionAtWrite(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	require.NoError(t, env.db.AutoMigrate(&model.Option{}))
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		drainImageEvidence(t)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	key := system_setting.TaskRequestEvidenceRetentionOptionKey
	require.NoError(t, model.UpdateOption(key, "7"))
	capture := newAPIRequestEvidence(model.TaskRequestEvidence{RequestID: "configured-retention"})
	record := capture.newRecord("north_receive", EvidenceObjectMeta{}, false)
	require.NotNil(t, record)
	record.append([]byte("request body"))
	record.close()
	capture.Finish(false)
	indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{RequestID: "configured-retention"})
	require.Len(t, indexes, 1)
	index := indexes[0]
	assert.Equal(t, index.CreatedAt+7*86400, index.RetainUntil)
	assert.False(t, IsEvidenceBodyExpired(index, index.CreatedAt+3*86400))
	require.NoError(t, model.UpdateOption(key, "2"))
	assert.True(t, IsEvidenceBodyExpired(index, index.CreatedAt+3*86400))
}

func TestEvidenceRetentionDeadline(t *testing.T) {
	day := int64(86400)
	created := int64(1000000)
	// Zero retention_until is bounded by the current retention window.
	assert.Equal(t, created+7*day, evidenceRetentionDeadline(created, 0, 7*day))
	// An earlier non-zero retain_until wins.
	assert.Equal(t, created+2*day, evidenceRetentionDeadline(created, created+2*day, 7*day))
	// Current window shorter than a historical longer period wins.
	assert.Equal(t, created+2*day, evidenceRetentionDeadline(created, created+7*day, 2*day))
}

func TestEvidenceBodyExpiredBoundaries(t *testing.T) {
	old := system_setting.GetTaskRequestEvidenceConfig()
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(old) })
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{RetentionDays: 2})
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
