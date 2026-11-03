package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceCreateDeliveryEvidencePreservesFundsAndPublicResponse(t *testing.T) {
	previousConfig := system_setting.GetTaskRequestEvidenceConfig()
	fx := newSeedanceFundsFixture(t)
	fx.assertAttemptAtPost.Store(true)
	require.NoError(t, fx.db.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.EvidenceTaskLink{}))
	config := system_setting.TaskRequestEvidenceConfig{
		Enabled: true, RetentionDays: 2, StorageDir: t.TempDir(),
		EncryptionKeyHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
	}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		flushControllerEvidence(t)
		require.NoError(t, service.InitTaskRequestEvidenceStore(previousConfig))
	})

	response := fx.submitCreate()
	flushControllerEvidence(t)
	publicID := decodeSeedanceFundsCreateID(t, response)
	task := fx.loadTask(publicID)
	assert.EqualValues(t, 1, fx.createCalls.Load())
	assert.Equal(t, string(fx.legacyOracleBody()), string(fx.createRequestBody()))
	assert.Equal(t, seedanceFundsInitialQuota-seedanceFundsHold, fx.userQuota())
	assert.Equal(t, seedanceFundsHold, task.Quota)
	assert.Equal(t, model.TaskBillingStatePending, task.BillingState)
	assert.Equal(t, "server_written", task.VideoDeliveryState)
	logs := fx.logsByType()
	require.Len(t, logs[model.LogTypeConsume], 1)
	assert.Equal(t, seedanceFundsHold, logs[model.LogTypeConsume][0].Quota)
	assert.Empty(t, logs[model.LogTypeRefund])
	var attempts []model.TaskCreateAttempt
	require.NoError(t, fx.db.Find(&attempts).Error)
	require.Len(t, attempts, 1)
	assert.Equal(t, model.TaskCreateAttemptComplete, attempts[0].Status)
	assert.Equal(t, model.TaskCreateAttemptBillingTransferred, attempts[0].BillingHoldState)

	var indexes []model.TaskRequestEvidence
	require.NoError(t, fx.db.Find(&indexes).Error)
	require.Len(t, indexes, 1)
	assert.Equal(t, task.TaskID, indexes[0].TaskID)
	assert.Equal(t, attempts[0].ID, indexes[0].AttemptID)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	var deliveries []*model.TaskRequestEvidenceEvent
	for _, event := range events {
		if event.Stage == model.TaskRequestEvidenceStageClientDelivery {
			deliveries = append(deliveries, event)
		}
	}
	require.Len(t, deliveries, 1)
	content, status := service.ReadEvidenceEventContent(deliveries[0], false)
	require.Equal(t, "available", status)
	assert.Equal(t, response, string(content.Body))
	assert.NotContains(t, string(content.Body), "prov-task-1")
	assert.NotContains(t, string(content.Body), "customer-video")
}
