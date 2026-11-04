package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorEventCleanupIsBoundedAndPreservesCutoffAndBilling(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, db.Create(&Log{Type: LogTypeConsume, Quota: 100, CreatedAt: 1}).Error)
	for _, at := range []int64{1, 2, 3, 100, 101} {
		require.NoError(t, db.Create(&ErrorEvent{CreatedAt: at}).Error)
	}
	count, err := DeleteOldErrorEventBatch(context.Background(), 100, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
	count, err = DeleteOldErrorEventBatch(context.Background(), 100, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	count, err = DeleteOldErrorEventBatch(context.Background(), 100, 2)
	require.NoError(t, err)
	assert.Zero(t, count)
	var remaining []int64
	require.NoError(t, db.Model(&ErrorEvent{}).Order("created_at").Pluck("created_at", &remaining).Error)
	assert.Equal(t, []int64{100, 101}, remaining)
	var log Log
	require.NoError(t, db.First(&log).Error)
	assert.Equal(t, 100, log.Quota)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = DeleteOldErrorEventBatch(ctx, 102, 2)
	require.ErrorIs(t, err, context.Canceled)
}
