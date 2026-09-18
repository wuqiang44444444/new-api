package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementParserUpgradeRejectsQueuedGeneration(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	ctx := context.Background()
	_, draft, err := model.AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NoError(t, db.Model(draft).Update("parser_version", model.BillingStatementParserVersion-1).Error)
	filters, err := common.Marshal(model.CustomerExportFilters{StatementDraftId: draft.DraftPublicId})
	require.NoError(t, err)
	err = runBillingStatementVersionGeneration(ctx, &model.CustomerExportJob{Filters: string(filters)}, &customerExportPressureTracker{})
	require.ErrorIs(t, err, model.ErrBillingStatementVersionConflict)
	current, err := model.GetBillingStatementVersion(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingStatementVersionQueued, current.Status)
	var count int64
	require.NoError(t, db.Model(&model.BillingStatementVersionLine{}).Where("version_id = ?", draft.ID).Count(&count).Error)
	assert.Zero(t, count)
}
