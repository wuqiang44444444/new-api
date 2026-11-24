package service

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportCSVIncludesSafeMeteredComponents(t *testing.T) {
	store := setupCustomerExportServiceTest(t)
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 1000000 // Changed since submission: export must use its frozen conversion.
	t.Cleanup(func() { common.QuotaPerUnit = previous })
	const userID = 42
	logs := []model.Log{
		{UserId: userID, Type: model.LogTypeConsume, CreatedAt: 13001, RequestId: "native", PromptTokens: 1000, Quota: 1000, Other: `{"model_ratio":1,"group_ratio":1,"contract_applicable":false,"admin_info":{"private":"must-not-export"}}`},
		{UserId: userID, Type: model.LogTypeConsume, CreatedAt: 13002, RequestId: "seconds", Quota: 1000000, Other: `{"billing_mode":"tiered_expr","matched_tier":"base","usage_units":{"seconds":"second"},"usage_facts":{"seconds":5},"group_ratio":1,"contract_applicable":false,"expr_b64":"` + base64.StdEncoding.EncodeToString([]byte(`tier("base", u("seconds") * 0.4)`)) + `"}`},
		{UserId: userID, Type: model.LogTypeConsume, CreatedAt: 13003, RequestId: "unknown", Quota: 5, Other: `{"matched_tier":"historical"}`},
		{UserId: userID, Type: model.LogTypeRefund, CreatedAt: 13004, RequestId: "refund", Quota: 5},
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)
	t.Cleanup(func() {
		model.LOG_DB.Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, 13000, 13100).Delete(&model.Log{})
	})
	filters := model.CustomerExportFilters{FieldVersion: customerExportFieldVersion, StartTimestamp: 13000, EndTimestamp: 13100, Timezone: "Asia/Shanghai", QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1}
	job, _, err := model.CreateCustomerExportJob(userID, userID, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	job, err = model.ClaimNextQueuedCustomerExportJob("explain", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	artifact, err := executeCustomerExportLogScan(context.Background(), job, filters, t.TempDir(), "explain", nil)
	require.NoError(t, err)
	require.Len(t, artifact.Files, 1)
	raw := string(store.uploaded[artifact.Files[0].ObjectKey])
	assert.NotContains(t, raw, "must-not-export")
	assert.NotContains(t, raw, "expr_b64")
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(raw, customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 5, "components must not duplicate ledger rows")
	rows := map[string]map[string]string{}
	for _, record := range records[1:] {
		row := reconciliationTestCells(t, adminUsageRecordColumns, record)
		rows[row["request_id"]] = row
	}
	assert.Equal(t, []string{
		"入账时间", "请求 ID", "入账类型", "客户 ID", "用户名", "令牌", "令牌 ID", "客户模型", "流",
		"净额 (USD)", "原价 (USD)", "入账额度", "折前额度", "是否计入客户账单", "合同 ID", "合同版本", "有效组倍率", "合同倍率",
		"模型最终倍率", "计费方式", "计入请求数", "输入 Token", "输出 Token", "缓存读取 Token", "普通写入 Token", "5 分钟写入 Token",
		"1 小时写入 Token", "12 小时写入 Token", "计费张数", "计费秒数", "用量及单价明细（USD，折扣前）", "金额无法计算原因",
		"任务合同应收 (USD)", "任务实际扣款 (USD)", "任务扣款差额 (USD)", "字段版本", "导出任务 ID", "币种", "单位金额对应额度", "汇率",
	}, records[0])
	assert.Equal(t, "28", rows["native"]["field_version"])
	assert.Equal(t, "否", rows["native"]["is_stream"])
	assert.Empty(t, rows["refund"]["is_stream"])
	assert.Contains(t, rows["native"]["billing_line_items_usd"], "1000 token; 2 USD / 1000000 token; 0.002 USD")
	assert.Contains(t, rows["seconds"]["billing_line_items_usd"], "5 second; 0.4 USD / 1 second; 2 USD")
	assert.Equal(t, "5", rows["seconds"]["billable_seconds"])
	assert.Empty(t, rows["unknown"]["billing_line_items_usd"])
	assert.Empty(t, rows["refund"]["billing_line_items_usd"])
	assert.Equal(t, "-0.00001000", rows["refund"]["net_amount"])

}

func TestCustomerExportWaitingTransitionsAreVisibleAndKeepCounters(t *testing.T) {
	setupCustomerExportServiceTest(t)
	filters := model.CustomerExportFilters{FieldVersion: customerExportFieldVersion, StartTimestamp: 13000, EndTimestamp: 13100}
	job, _, err := model.CreateCustomerExportJob(42, 42, model.CustomerExportJobTypeUsageLogs, filters)
	require.NoError(t, err)
	job, err = model.ClaimNextQueuedCustomerExportJob("wait", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	progress := model.CustomerExportProgress{Scanned: 1000, Matched: 10, Written: 10, Files: 1}
	pressure := &customerExportPressureTracker{degraded: true}
	for _, waiting := range []bool{true, false} {
		pressure.degraded = waiting
		require.NoError(t, pressure.publishWaiting(context.Background(), job.JobID, "wait", &progress))
		stored, err := model.GetCustomerExportJob(job.JobID)
		require.NoError(t, err)
		assert.Equal(t, waiting, stored.ToView().Progress.WaitingE)
		assert.EqualValues(t, 1000, stored.ToView().Progress.Scanned)
		assert.EqualValues(t, 10, stored.ToView().Progress.Written)
	}
}
