package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkCachePendingLogFencesOnlyItsStatementPeriod(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	priorDataExportEnabled := common.DataExportEnabled
	common.DataExportEnabled = false
	t.Cleanup(func() { common.DataExportEnabled = priorDataExportEnabled })
	ctx := t.Context()
	const userID, period = 11, 1756656000
	_, draft, err := AcquireBillingStatementDraft(ctx, userID, period, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, userID, period, draft)
	row := ArkPrefixCache{ID: ArkSessionCacheIDPrefix + "staged-log", UserID: userID, KeyDigest: ArkSessionCacheKeyDigest("staged-log"),
		State: ArkCacheActive, SessionRound: 1, CreatedAt: period + 60, InferenceLogDueAt: common.GetTimestamp() - 1,
		InferenceQuota: 9, InferenceLog: `{"billing_source":"wallet"}`}
	require.NoError(t, DB.Create(&row).Error)
	require.NoError(t, verifyArkCacheStatementSource(DB, userID+1, period, period+3600))
	require.NoError(t, verifyArkCacheStatementSource(DB, userID, period+3600, period+7200))
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "pending", "", "", 7)
	require.ErrorIs(t, err, ErrBillingStatementSourceIncomplete)
	assert.False(t, committed)

	require.NoError(t, DeliverArkSessionLog(ctx, row.ID))
	require.NoError(t, verifyArkCacheStatementSource(DB, userID, period, period+3600))
	_, committed, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "stale", "", "", 7)
	require.Error(t, err, "publishing the log must invalidate the pre-delivery draft")
	assert.False(t, committed)
}
