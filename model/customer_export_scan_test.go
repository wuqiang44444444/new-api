package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextCustomerExportLogBatchKeysetNoLoss(t *testing.T) {
	setupCustomerExportTestDB(t)

	logs := make([]Log, 0, 50)
	for i := 0; i < 50; i++ {
		logs = append(logs, Log{
			UserId: 21, CreatedAt: int64(6000 + i), Type: LogTypeConsume,
			TokenId: 3, TokenName: "key", ModelName: "model-a", Quota: 10 + i,
		})
	}
	// 其他用户的行与范围外的行都不应进入扫描结果。
	logs = append(logs,
		Log{UserId: 22, CreatedAt: 6050, Type: LogTypeConsume, TokenId: 3, TokenName: "other", ModelName: "model-a", Quota: 1},
		Log{UserId: 21, CreatedAt: 5999, Type: LogTypeConsume, TokenId: 3, TokenName: "key", ModelName: "model-a", Quota: 1},
		Log{UserId: 21, CreatedAt: 6100, Type: LogTypeConsume, TokenId: 3, TokenName: "key", ModelName: "model-a", Quota: 1},
	)
	require.NoError(t, DB.Create(&logs).Error)

	params := CustomerExportBatchParams{UserId: 21, StartTimestamp: 6000, EndTimestamp: 6100, Limit: 20}
	seen := make(map[int]bool)
	batches := 0
	cursor := int64(0)
	for {
		page, err := NextCustomerExportLogBatch(testContext(), withScanCursor(params, cursor))
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		require.LessOrEqual(t, len(page), 20)
		lastId := 0
		for _, row := range page {
			assert.False(t, seen[row.Id], "row %d must not repeat", row.Id)
			seen[row.Id] = true
			assert.Greater(t, row.Id, lastId)
			lastId = row.Id
		}
		cursor = int64(lastId)
		batches++
		if len(page) < 20 {
			break
		}
	}
	assert.Equal(t, 3, batches)
	assert.Len(t, seen, 50)
}

func withScanCursor(params CustomerExportBatchParams, cursor int64) CustomerExportBatchParams {
	params.CursorId = cursor
	return params
}

func testContext() context.Context { return context.Background() }

func TestBuildCustomerExportRowsDiscountFacts(t *testing.T) {
	setupCustomerExportTestDB(t)

	logs := []Log{
		// 折扣齐备：组 0.5 × 合同 0.3，折前金额为估算。
		{UserId: 31, CreatedAt: 7000, Type: LogTypeConsume, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 150,
			Other: `{"group_ratio":0.5,"contract_discount":"0.3","contract_version":3,"contract_name":"年度合同"}`},
		// 结算时明确未适用合同。
		{UserId: 31, CreatedAt: 7001, Type: LogTypeConsume, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 100,
			Other: `{"group_ratio":0.5,"contract_applicable":false}`},
		// 历史行：合同信息缺失 → 未记录，不是“未适用”。
		{UserId: 31, CreatedAt: 7002, Type: LogTypeConsume, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 100,
			Other: `{"group_ratio":0.5}`},
		// 用户专属组倍率：只显示最终生效的一项。
		{UserId: 31, CreatedAt: 7003, Type: LogTypeConsume, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 80,
			Other: `{"group_ratio":0.5,"user_group_ratio":0.8,"contract_applicable":false}`},
		// 附加收费：折前金额不可反推。
		{UserId: 31, CreatedAt: 7004, Type: LogTypeConsume, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 90,
			Other: `{"group_ratio":0.5,"tool_surcharges":{"search":10}}`},
		// 退款行：估算原价保留负号。
		{UserId: 31, CreatedAt: 7005, Type: LogTypeRefund, TokenId: 1, TokenName: "key", ModelName: "m-a", Quota: 30,
			Other: `{"group_ratio":0.5,"contract_discount":"0.3","contract_name":"年度合同"}`},
		// 渠道测试行：不计入客户账单。
		{UserId: 31, CreatedAt: 7006, Type: LogTypeConsume, TokenId: 0, TokenName: "模型测试", ModelName: "m-a", Quota: 5, Content: "模型测试"},
	}
	require.NoError(t, DB.Create(&logs).Error)

	page, err := NextCustomerExportLogBatch(testContext(), CustomerExportBatchParams{UserId: 31, StartTimestamp: 7000, EndTimestamp: 7007, Limit: 100})
	require.NoError(t, err)
	require.Len(t, page, 7)

	rows, err := BuildCustomerExportRows(testContext(), page, "", "", nil)
	require.NoError(t, err)
	require.Len(t, rows, 7)

	first := rows[0]
	assert.Equal(t, "yes", first.ContractApplicable)
	assert.Equal(t, "年度合同", first.ContractName)
	require.NotNil(t, first.GroupRatio)
	assert.InDelta(t, 0.5, *first.GroupRatio, 1e-9)
	require.NotNil(t, first.ContractRatio)
	assert.InDelta(t, 0.3, *first.ContractRatio, 1e-9)
	require.NotNil(t, first.FinalRatio)
	assert.InDelta(t, 0.15, *first.FinalRatio, 1e-6)
	assert.Equal(t, "estimate", first.QualityStatus)
	assert.Equal(t, "1000.0000", first.OriginalEstimate)

	assert.Equal(t, "no", rows[1].ContractApplicable)
	assert.Empty(t, rows[1].ContractName)
	assert.Equal(t, "unrecorded", rows[2].ContractApplicable)
	assert.Equal(t, "estimate", rows[2].QualityStatus)
	assert.Equal(t, "200.0000", rows[2].OriginalEstimate)
	require.NotNil(t, rows[2].FinalRatio)
	assert.Equal(t, 0.5, *rows[2].FinalRatio)

	require.NotNil(t, rows[3].GroupRatio)
	assert.InDelta(t, 0.8, *rows[3].GroupRatio, 1e-9)
	assert.Equal(t, "user_exclusive", rows[3].GroupRatioSource)

	assert.Equal(t, "unrecorded", rows[4].QualityStatus)
	assert.True(t, rows[4].HasAuxiliaryCharge)

	require.NotNil(t, rows[5].FinalRatio)
	assert.Equal(t, "-200.0000", rows[5].OriginalEstimate)
	assert.Equal(t, "refund", rows[5].EventType)

	assert.False(t, rows[6].CountsTowardBill)
	assert.Equal(t, "unrecorded", rows[6].QualityStatus)

	// 客户模型过滤：候选行 7，匹配行按解析后的客户模型计数。
	filtered, err := BuildCustomerExportRows(testContext(), page, "m-a", "", nil)
	require.NoError(t, err)
	assert.Len(t, filtered, 7)
	none, err := BuildCustomerExportRows(testContext(), page, "other-model", "", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestNextCustomerExportLogBatchStatementScopeExcludesChannelTests(t *testing.T) {
	setupCustomerExportTestDB(t)

	logs := []Log{
		{UserId: 41, CreatedAt: 8000, Type: LogTypeConsume, TokenId: 2, TokenName: "key", ModelName: "m-a", Quota: 10},
		{UserId: 41, CreatedAt: 8001, Type: LogTypeConsume, TokenId: 0, TokenName: "模型测试", ModelName: "m-a", Quota: 5, Content: "模型测试"},
		{UserId: 41, CreatedAt: 8002, Type: LogTypeRefund, TokenId: 2, TokenName: "key", ModelName: "m-a", Quota: 2},
	}
	require.NoError(t, DB.Create(&logs).Error)

	details, err := NextCustomerExportLogBatch(testContext(), CustomerExportBatchParams{
		UserId: 41, StartTimestamp: 8000, EndTimestamp: 8003, Limit: 100,
		LogTypes: []int{LogTypeConsume, LogTypeRefund}, StatementScope: true,
	})
	require.NoError(t, err)
	require.Len(t, details, 2)
	for _, row := range details {
		assert.NotEqual(t, "模型测试", row.TokenName)
	}
}
