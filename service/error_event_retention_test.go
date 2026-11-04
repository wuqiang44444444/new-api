package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorEventRetentionUsesDurableCutoffAndCanBeDisabled(t *testing.T) {
	truncate(t)
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.ErrorEvent{}))
	t.Cleanup(func() { model.LOG_DB.Where("request_id = ?", "retention-fixture").Delete(&model.ErrorEvent{}) })
	t.Setenv("ERROR_EVENTS_RETENTION_DAYS", "0")
	handler := errorEventRetentionHandler{}
	assert.False(t, handler.Enabled())
	t.Setenv("ERROR_EVENTS_RETENTION_DAYS", "30")
	assert.True(t, handler.Enabled())
	payload := struct {
		Cutoff int64 `json:"cutoff"`
	}{100}
	task, err := model.CreateSystemTask(handler.Type(), payload, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, handler.Type(), "retention-test", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, model.LOG_DB.Create(&[]model.ErrorEvent{{RequestId: "retention-fixture", CreatedAt: 99}, {RequestId: "retention-fixture", CreatedAt: 100}}).Error)
	handler.Run(context.Background(), claimed, "retention-test")
	finished, err := model.GetLatestSystemTask(handler.Type())
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	var remaining []int64
	require.NoError(t, model.LOG_DB.Model(&model.ErrorEvent{}).Where("request_id = ?", "retention-fixture").Pluck("created_at", &remaining).Error)
	assert.Equal(t, []int64{100}, remaining, "execution must keep its submitted cutoff instead of recomputing today's retention")
}
