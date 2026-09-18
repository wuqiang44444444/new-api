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
		row := map[string]string{}
		for i, key := range records[0] {
			row[key] = record[i]
		}
		rows[row["request_id"]] = row
	}
	for _, tc := range []struct {
		id, unit, tier, basis                    string
		quantity, price, subtotal, priceQuantity float64
	}{
		{"native", "token", "", "frozen_quota_conversion", 1000, 2, .002, 1000000},
		{"seconds", "second", "base", "recorded_usd_price", 5, .4, 2, 1},
	} {
		row := rows[tc.id]
		assert.Equal(t, "available", row["billing_explanation_status"])
		assert.Equal(t, tc.tier, row["matched_tier"])
		assert.Equal(t, tc.basis, row["price_conversion_basis"])
		var lines []map[string]any
		require.NoError(t, common.UnmarshalJsonStr(row["billing_line_items_usd"], &lines))
		require.Len(t, lines, 1)
		assert.Equal(t, tc.quantity, lines[0]["quantity"])
		assert.Equal(t, tc.unit, lines[0]["unit"])
		assert.Equal(t, tc.price, lines[0]["unit_price_usd"])
		assert.Equal(t, tc.priceQuantity, lines[0]["unit_price_quantity"])
		assert.InDelta(t, tc.subtotal, lines[0]["subtotal_usd"], 1e-10)
	}
	assert.Equal(t, "unavailable", rows["unknown"]["billing_explanation_status"])
	assert.Empty(t, rows["unknown"]["billing_line_items_usd"])
	assert.Equal(t, "historical", rows["unknown"]["matched_tier"])
	assert.Equal(t, "not_applicable", rows["refund"]["billing_explanation_status"])
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
