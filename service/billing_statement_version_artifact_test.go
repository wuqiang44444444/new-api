package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func versionFixtureStatement(t *testing.T, db *gorm.DB, draftPublicId string, netQuota int64) *model.BillingStatementVersion {
	t.Helper()
	ctx := context.Background()
	_, draft, err := model.AcquireBillingStatementDraft(ctx, 5001, 1756608000, "Asia/Shanghai", 9)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.BillingStatementVersion{}).Where("id = ?", draft.ID).Update("draft_public_id", draftPublicId).Error)
	snapshot, err := model.FreezeBillingStatementProjection(model.BillingCustomerStatement{Summary: model.BillingReconciliationUsage{Requests: 2, GrossQuota: netQuota + 100, RefundQuota: 100, NetQuota: netQuota}, Groups: []model.BillingReconciliationGroupSummary{{Id: 7, Name: "key-7", Usage: model.BillingReconciliationUsage{NetQuota: netQuota}, Models: []model.BillingReconciliationModelSummary{{ModelName: "m1", BillingMode: model.BillingReconciliationModePerCall, Usage: model.BillingReconciliationUsage{NetQuota: netQuota}}}}}})
	require.NoError(t, err)
	require.NoError(t, db.Model(draft).Update("summary_projection", snapshot).Error)
	require.NoError(t, db.Model(draft).Updates(map[string]interface{}{"quota_per_unit": 500000, "currency": "USD", "currency_rate": 1}).Error)
	v, err := model.GetBillingStatementVersionByDraftPublicId(ctx, draftPublicId)
	require.NoError(t, err)
	// 释放活动草稿指针，允许同客户月继续造下一个版本。
	require.NoError(t, model.AbandonBillingStatementDraft(ctx, draftPublicId, 9, "fixture"))
	return v
}

// TestComputeBillingStatementVersionDiff 验证版本差异全部来自冻结投影：
// 金额差、分组差与质量计数差，未知侧不产生差值。
func TestComputeBillingStatementVersionDiff(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	_ = db
	base := versionFixtureStatement(t, db, "bsv_diff_base", 400)
	compare := versionFixtureStatement(t, db, "bsv_diff_compare", 460)

	diff, err := ComputeBillingStatementVersionDiff(base, compare)
	require.NoError(t, err)
	assert.EqualValues(t, base.ID, diff.BaseVersionId)
	assert.EqualValues(t, compare.ID, diff.CompareVersionId)

	net := diff.Items["net_quota"]
	require.NotNil(t, net.Base)
	require.NotNil(t, net.Compare)
	require.NotNil(t, net.Delta)
	assert.EqualValues(t, 400, *net.Base)
	assert.EqualValues(t, 460, *net.Compare)
	assert.EqualValues(t, 60, *net.Delta)

	// 分组与模型行差值同源。
	require.Len(t, diff.Groups, 1)
	assert.EqualValues(t, 60, *diff.Groups[0].Net.Delta)
	require.Len(t, diff.Models, 1)
	assert.Equal(t, "m1", diff.Models[0].ModelName)
	assert.EqualValues(t, 60, *diff.Models[0].Net.Delta)
}

func TestComputeBillingStatementVersionDiffIncludesNonMonetaryCorrections(t *testing.T) {
	left := model.BillingCustomerStatement{
		Summary:              model.BillingReconciliationUsage{InputTokens: 0, CacheWriteTokens: 1, NetQuota: 100},
		DataQuality:          &model.BillingReconciliationDataQuality{InputTokensUnavailableRequests: 1},
		DiscountCombinations: []model.BillingDiscountCombination{{ModelName: "customer-model", ContractApplicable: "no"}},
	}
	right := model.BillingCustomerStatement{
		Summary:              model.BillingReconciliationUsage{InputTokens: 9007199254740993, CacheWriteTokens: 3, NetQuota: 100},
		DiscountCombinations: []model.BillingDiscountCombination{{ModelName: "customer-model", ContractApplicable: "yes", ContractIdKnown: true, ContractId: 7, ContractVersion: 2, ContractName: "Historical contract"}},
	}
	leftJSON, err := model.FreezeBillingStatementProjection(left)
	require.NoError(t, err)
	rightJSON, err := model.FreezeBillingStatementProjection(right)
	require.NoError(t, err)
	diff, err := ComputeBillingStatementVersionDiff(&model.BillingStatementVersion{SummaryProjection: leftJSON, QuotaPerUnit: 100}, &model.BillingStatementVersion{SummaryProjection: rightJSON, QuotaPerUnit: 100})
	require.NoError(t, err)
	require.NotNil(t, diff.Items["net_quota"].Delta)
	assert.Zero(t, *diff.Items["net_quota"].Delta)
	assert.Nil(t, diff.Usage["input_tokens"].Base)
	assert.Nil(t, diff.Usage["input_tokens"].Delta)
	require.NotNil(t, diff.Usage["input_tokens"].Compare)
	assert.Equal(t, "9007199254740993", *diff.Usage["input_tokens"].Compare)
	require.NotNil(t, diff.Usage["cache_write_tokens"].Delta)
	assert.Equal(t, "2", *diff.Usage["cache_write_tokens"].Delta)
	assert.EqualValues(t, -1, *diff.Quality["input_tokens_unavailable_requests"].Delta)
	assert.Equal(t, left.DiscountCombinations, diff.Discounts.Base)
	assert.Equal(t, right.DiscountCombinations, diff.Discounts.Compare)
}
