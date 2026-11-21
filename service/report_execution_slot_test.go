package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedReportSlotBlocksOverlapAndStopsNextBatchUnderPressure(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTaskLock{}))
	ctx := t.Context()
	entered, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	admit := func(context.Context) error { return nil }
	go func() {
		result <- executeSharedReportBatch(ctx, admit, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case err := <-result:
		t.Fatalf("first background batch failed: %v", err)
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	err := executeSharedReportBatch(ctx, admit, func(context.Context) error { return assert.AnError })
	close(release)
	require.ErrorIs(t, err, errReportBatchDeferred)
	require.NoError(t, <-result)
	block := func(context.Context) error { return model.ErrDailyReportResourcePressure }
	require.ErrorIs(t, executeSharedReportBatch(ctx, block, func(context.Context) error { return assert.AnError }), errReportBatchDeferred)
	require.NoError(t, executeSharedReportBatch(ctx, admit, func(context.Context) error { return nil }))
}
