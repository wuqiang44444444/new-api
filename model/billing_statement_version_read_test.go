package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := common.Marshal(v)
	require.NoError(t, err)
	return string(data)
}

// TestAcquireCorrectionDraftRequiresCurrentBase 验证更正草稿占用：基准必须是当前确认版，
// 且同月只能有一个活动草稿。
func TestAcquireCorrectionDraftRequiresCurrentBase(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()

	// 未确认任何版本时不能发起更正。
	_, _, err := AcquireBillingStatementCorrectionDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 999, "r", "n", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)

	// 确认 v1。
	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, draft)
	v1, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-c1", "qa", "", 7)
	require.NoError(t, err)
	assert.True(t, committed)

	// 基准不是当前确认版：拒绝。
	_, _, err = AcquireBillingStatementCorrectionDraft(ctx, 1001, 1756608000, "Asia/Shanghai", v1.ID+100, "r", "n", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)

	// 正确基准：占用成功，原因与备注冻结在草稿上。
	_, correction, err := AcquireBillingStatementCorrectionDraft(ctx, 1001, 1756608000, "Asia/Shanghai", v1.ID, "补入漏记记录", "内部说明", 7)
	require.NoError(t, err)
	require.NotNil(t, correction.CorrectsVersionId)
	assert.Equal(t, v1.ID, *correction.CorrectsVersionId)
	assert.Equal(t, "补入漏记记录", correction.PublicReason)
	assert.Equal(t, "内部说明", correction.InternalNote)

	// 活动草稿存在时不能再次占用。
	_, _, err = AcquireBillingStatementCorrectionDraft(ctx, 1001, 1756608000, "Asia/Shanghai", v1.ID, "r", "n", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

// TestCleanupDraftGuardsAndDeletes 验证草稿清理：仅失效/失败/取消可清理，
// 清理中草稿事实被删除，可确认草稿不会被误标。
func TestCleanupDraftGuardsAndDeletes(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()

	// 待确认草稿不可清理。
	_, pending, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, pending)
	marked, err := MarkBillingStatementDraftCleaning(ctx, pending.DraftPublicId, 7)
	require.NoError(t, err)
	assert.False(t, marked)
	still, err := GetBillingStatementVersionByDraftPublicId(ctx, pending.DraftPublicId)
	require.NoError(t, err)
	assert.Equal(t, BillingStatementVersionPending, still.Status)

	// 失效草稿可清理：标记后事实删除。
	_, invalid, err := AcquireBillingStatementDraft(ctx, 1002, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NoError(t, AbandonBillingStatementDraft(ctx, invalid.DraftPublicId, 7, "test"))
	marked, err = MarkBillingStatementDraftCleaning(ctx, invalid.DraftPublicId, 7)
	require.NoError(t, err)
	assert.True(t, marked)

	keys, err := DeleteBillingStatementDraftFactsTx(ctx, db, invalid.DraftPublicId)
	require.NoError(t, err)
	assert.Empty(t, keys)
	_, err = GetBillingStatementVersionByDraftPublicId(ctx, invalid.DraftPublicId)
	assert.Error(t, err) // 已删除

	// 月记录活动草稿指针已释放（放弃时），可重新占用。
	_, again, err := AcquireBillingStatementDraft(ctx, 1002, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	assert.NotEqual(t, invalid.ID, again.ID)
}

// TestSetBillingStatementVersionEnabledPersistsOption 验证开关切换经过维护控制点并持久化。
func TestSetBillingStatementVersionEnabledPersistsOption(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	ctx := context.Background()

	require.NoError(t, SetBillingStatementVersionEnabled(ctx, true, 7))
	assert.True(t, BillingStatementVersionEnabled())
	var option Option
	require.NoError(t, DB.Where("key = ?", BillingStatementVersionEnabledKey).First(&option).Error)
	assert.Equal(t, "true", option.Value)

	require.NoError(t, SetBillingStatementVersionEnabled(ctx, false, 7))
	assert.False(t, BillingStatementVersionEnabled())
	require.NoError(t, DB.Where("key = ?", BillingStatementVersionEnabledKey).First(&option).Error)
	assert.Equal(t, "false", option.Value)
}

// TestVersionStatementReconstruction 验证冻结投影可重建账单视图（读路径绑定基础）。
func TestVersionStatementReconstruction(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	ctx := context.Background()

	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	snapshot, err := FreezeBillingStatementProjection(BillingCustomerStatement{Summary: BillingReconciliationUsage{Requests: 3, GrossQuota: 500, RefundQuota: 100, NetQuota: 400}, Groups: []BillingReconciliationGroupSummary{{Id: 7, Name: "key-7"}}})
	require.NoError(t, err)
	require.NoError(t, DB.Model(draft).Update("summary_projection", snapshot).Error)

	v, err := GetBillingStatementVersionByDraftPublicId(ctx, draft.DraftPublicId)
	require.NoError(t, err)
	statement, err := BillingStatementVersionStatement(v)
	require.NoError(t, err)
	assert.EqualValues(t, 400, statement.Summary.NetQuota)
	require.Len(t, statement.Groups, 1)
	assert.EqualValues(t, 7, statement.Groups[0].Id)
	assert.Equal(t, "key-7", statement.Groups[0].Name)
}
