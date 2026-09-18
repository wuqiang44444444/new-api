package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementParserUpgradeRejectsPendingDraft(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	const period = int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, period, draft)
	require.NoError(t, DB.Model(draft).Update("parser_version", BillingStatementParserVersion-1).Error)
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "old", "", "", 7)
	require.ErrorIs(t, err, ErrBillingStatementVersionConflict)
	assert.False(t, committed)
	month, err := GetBillingStatementMonthByUserPeriod(ctx, 11, period)
	require.NoError(t, err)
	assert.Nil(t, month.CurrentVersionId)
	assert.Nil(t, month.ActiveDraftId)
	invalid, err := GetBillingStatementVersion(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, BillingStatementVersionInvalid, invalid.Status)
	_, fresh, err := AcquireBillingStatementDraft(ctx, 11, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, period, fresh)
	confirmed, committed, err := ConfirmBillingStatementVersion(ctx, fresh.DraftPublicId, nil, "new", "", "", 7)
	require.NoError(t, err)
	assert.True(t, committed)
	assert.Equal(t, BillingStatementParserVersion, confirmed.ParserVersion)
}

func TestBillingStatementParserUpgradePreservesConfirmedFacts(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	const period = int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, period, draft)
	confirmed, _, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "first", "", "", 7)
	require.NoError(t, err)
	// Simulate a previously published interpretation whose savings cannot be
	// replaced by today's formula while retaining the same version number.
	original, frozenSavings := int64(100), int64(17)
	projection, err := FreezeBillingStatementProjection(BillingCustomerStatement{
		Groups: []BillingReconciliationGroupSummary{{Models: []BillingReconciliationModelSummary{{
			OriginalQuota: &original, DiscountQuota: &frozenSavings,
			Usage: BillingReconciliationUsage{NetQuota: 80},
		}}}},
	})
	require.NoError(t, err)
	require.NoError(t, DB.Model(confirmed).Updates(map[string]any{"parser_version": BillingStatementParserVersion - 1, "summary_projection": projection}).Error)
	retry, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "retry", "", "", 7)
	require.NoError(t, err)
	assert.False(t, committed)
	assert.Equal(t, confirmed.ID, retry.ID)
	view, err := BillingStatementVersionStatement(retry)
	require.NoError(t, err)
	require.Len(t, view.Groups, 1)
	require.Len(t, view.Groups[0].Models, 1)
	assert.Equal(t, &frozenSavings, view.Groups[0].Models[0].DiscountQuota)
	_, correction, err := AcquireBillingStatementCorrectionDraft(ctx, 11, period, "Asia/Shanghai", retry.ID, "verified correction", "", 7)
	require.NoError(t, err)
	assert.Equal(t, BillingStatementParserVersion, correction.ParserVersion)
}
