package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A task whose frozen plugin version no longer resolves must stay fail-closed:
// the poll parks the task in reconciliation instead of silently falling back
// to the active artifact, which would reinterpret an already frozen contract.
func TestSynlinkFrozenVersionMissingDoesNotFallbackToActive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}))
	original := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = original
		sqlDB, dbErr := db.DB()
		require.NoError(t, dbErr)
		require.NoError(t, sqlDB.Close())
	})

	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:  dto.VideoUpstreamProfileThirdPartySynlinkVideoV1,
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1,
		Execution: &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{
			Key: SeedanceExtensionPluginKey, Version: "0.0.0-test-missing", APIVersion: 3,
		}},
	}}
	_, err = normalizeSeedanceVideoTaskResponse(
		context.Background(), task, dto.VideoUpstreamProfileThirdPartySynlinkVideoV1,
		relaycommon.VideoSouthboundAdapterVersion{},
		[]byte(`{"task":{"id":"syn-1","status":"processing"}}`), "syn-1", "https://provider.example", nil,
	)
	require.Error(t, err)
	var violation *relaycommon.UpstreamContractViolation
	require.ErrorAs(t, err, &violation)
	assert.Contains(t, violation.Reason, `seedance plugin version "0.0.0-test-missing" is unavailable`)
}
