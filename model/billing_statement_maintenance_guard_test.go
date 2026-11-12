package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingStatementOfflineRepairInvalidatesPendingAndExportReuse(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	ctx := context.Background()
	const period = int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, period, draft)
	filters := CustomerExportFilters{StartTimestamp: period, EndTimestamp: period + 100}
	before, err := CustomerStatementExportSourceVersion(ctx, 11, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	require.NotEmpty(t, before)
	maintenance, err := BeginBillingStatementMaintenance(ctx, "approved offline correction", 7)
	require.NoError(t, err)
	_, _, err = AcquireBillingStatementDraft(ctx, 12, period, "Asia/Shanghai", 7)
	require.ErrorIs(t, err, ErrBillingStatementMaintenanceActive)
	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "during", "", "", 7)
	require.ErrorIs(t, err, ErrBillingStatementMaintenanceActive)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return RequireBillingStatementMaintenanceTx(tx, maintenance.Generation)
	}))
	require.Error(t, db.Transaction(func(tx *gorm.DB) error {
		return RequireBillingStatementMaintenanceTx(tx, maintenance.Generation-1)
	}))
	_, err = EndBillingStatementMaintenance(ctx, maintenance.Generation, "verified correction evidence", 7)
	require.NoError(t, err)
	require.Error(t, db.Transaction(func(tx *gorm.DB) error {
		return RequireBillingStatementMaintenanceTx(tx, maintenance.Generation)
	}))
	_, _, err = AcquireBillingStatementDraft(ctx, 12, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "after", "", "", 7)
	require.ErrorIs(t, err, ErrBillingStatementVersionConflict)
	assert.False(t, committed)
	after, err := CustomerStatementExportSourceVersion(ctx, 11, CustomerExportJobTypeStatementSummary, filters)
	require.NoError(t, err)
	assert.NotEqual(t, before, after)
}
