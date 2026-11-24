package service

import (
	"encoding/csv"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementExportOmitsUnknownAmountsWhileUsageExportRetainsReasons(t *testing.T) {
	reasons := []string{model.BillingEstimateMissingContract}
	usage := model.BillingReconciliationUsage{GrossQuota: 100, NetQuota: 100}
	statement := model.BillingCustomerStatement{
		Summary: usage, EstimateReasons: reasons,
		Groups: []model.BillingReconciliationGroupSummary{{Id: 4, Name: "key", Usage: usage, EstimateReasons: reasons,
			Models: []model.BillingReconciliationModelSummary{{ModelName: "model", Usage: usage, EstimateReasons: reasons}}}},
		DiscountCombinations: []model.BillingDiscountCombination{{GroupId: 4, ModelName: "model", Usage: usage, EstimateReasons: reasons}},
	}
	scope := customerExportScopeColumns{QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1, Language: "en"}
	for _, language := range []string{"en", "zh"} {
		t.Run(language, func(t *testing.T) {
			_, paths, _, err := writeCustomerExportSummaryCsv(t.TempDir(), scope, language, statement)
			require.NoError(t, err)
			require.Len(t, paths, 1)
			records := readEstimateCsv(t, paths[0])
			require.Len(t, records, 2)
			values := reconciliationTestCells(t, customerSummaryColumns, records[1])
			assert.Empty(t, values["original_amount"])
			assert.Equal(t, "0.00020000", values["net_amount"])
			assert.NotEmpty(t, values["notes"])
		})
	}
	writer, err := newCustomerExportCsvWriter(t.TempDir(), "detail", 1<<20)
	require.NoError(t, err)
	require.NoError(t, writer.AppendRow(model.CustomerExportRow{EventType: "consume", Quota: 100, EstimateReasons: reasons}, scope))
	require.NoError(t, writer.AppendRow(model.CustomerExportRow{EventType: "consume", Quota: 0, RecordedOriginal: "0", OriginalExact: true}, scope))
	_, paths, _, _, err := writer.Finish()
	require.NoError(t, err)
	records := readEstimateCsv(t, paths[0])
	first := reconciliationTestCells(t, adminUsageRecordColumns, records[1])
	second := reconciliationTestCells(t, adminUsageRecordColumns, records[2])
	assert.Equal(t, "无法计算：历史合同状态未记录", first["amount_unavailable_reasons"])
	assert.Empty(t, first["original_amount"])
	assert.Empty(t, second["amount_unavailable_reasons"])
	assert.Equal(t, "0", second["original_quota"])
	assert.Equal(t, "0.00000000", second["original_amount"])
}

func readEstimateCsv(t *testing.T, path string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	return rows
}

func TestCustomerExportModelSavingsMatchOriginalMinusNet(t *testing.T) {
	original, discount := int64(200), int64(100)
	scope := customerExportScopeColumns{QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1, Language: "en"}
	statement := model.BillingCustomerStatement{Groups: []model.BillingReconciliationGroupSummary{{Id: 4, Models: []model.BillingReconciliationModelSummary{
		{ModelName: "known", OriginalQuota: &original, DiscountQuota: &discount, Usage: model.BillingReconciliationUsage{NetQuota: 100}},
		{ModelName: "unknown", Usage: model.BillingReconciliationUsage{NetQuota: 100}},
	}}}}
	_, paths, _, err := writeCustomerExportSummaryCsv(t.TempDir(), scope, "en", statement)
	require.NoError(t, err)
	rows := readEstimateCsv(t, paths[0])
	seen := map[string]string{}
	for _, row := range rows[1:] {
		cells := reconciliationTestCells(t, customerSummaryColumns, row)
		seen[cells["model"]] = cells["discount_amount"]
	}
	assert.Equal(t, map[string]string{"known": "0.00020000", "unknown": ""}, seen)
}

func reconciliationTestCells(t *testing.T, columns []reconciliationCSVColumn, row []string) map[string]string {
	t.Helper()
	require.Len(t, row, len(columns))
	values := make(map[string]string, len(columns))
	for index, column := range columns {
		values[column.key] = row[index]
	}
	return values
}
