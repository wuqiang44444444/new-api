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

func TestCustomerExportEstimateReasonsInSummaryAndDetails(t *testing.T) {
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
			header := map[string]int{}
			for i, name := range records[0] {
				header[name] = i
			}
			reasonColumn := customerExportSummaryHeaderLabel(language, "estimate_reasons")
			require.Contains(t, header, reasonColumn)
			require.Len(t, records, 2)
			for _, row := range records[1:] {
				expectedReason := "Cannot estimate: historical contract status not recorded"
				if language == "zh" {
					expectedReason = "无法估算：历史合同状态未记录"
				}
				assert.Equal(t, expectedReason, row[header[reasonColumn]])
				assert.Empty(t, row[header[customerExportSummaryHeaderLabel(language, "original_amount")]])
				assert.Equal(t, "0.00020000", row[header[customerExportSummaryHeaderLabel(language, "net_amount")]])
			}
		})
	}
	writer, err := newCustomerExportCsvWriter(t.TempDir(), "detail", 1<<20)
	require.NoError(t, err)
	require.NoError(t, writer.AppendRow(model.CustomerExportRow{EventType: "consume", Quota: 100, EstimateReasons: reasons}, scope))
	require.NoError(t, writer.AppendRow(model.CustomerExportRow{EventType: "consume", Quota: 0, OriginalEstimate: "0", OriginalExact: true}, scope))
	_, paths, _, _, err := writer.Finish()
	require.NoError(t, err)
	records := readEstimateCsv(t, paths[0])
	header := map[string]int{}
	for i, name := range records[0] {
		header[name] = i
	}
	require.Contains(t, header, "estimate_reasons")
	assert.Equal(t, "Cannot estimate: historical contract status not recorded", records[1][header["estimate_reasons"]])
	assert.Empty(t, records[1][header["original_amount_estimated"]])
	assert.Empty(t, records[2][header["estimate_reasons"]])
	assert.Equal(t, "0", records[2][header["original_estimate_quota"]])
	assert.NotEmpty(t, records[2][header["discount_amount_estimated"]])
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
	original := int64(200)
	scope := customerExportScopeColumns{QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1, Language: "en"}
	statement := model.BillingCustomerStatement{Groups: []model.BillingReconciliationGroupSummary{{Id: 4, Models: []model.BillingReconciliationModelSummary{
		{ModelName: "known", OriginalQuota: &original, Usage: model.BillingReconciliationUsage{NetQuota: 100}},
		{ModelName: "unknown", Usage: model.BillingReconciliationUsage{NetQuota: 100}},
	}}}}
	_, paths, _, err := writeCustomerExportSummaryCsv(t.TempDir(), scope, "en", statement)
	require.NoError(t, err)
	rows := readEstimateCsv(t, paths[0])
	header := map[string]int{}
	for i, label := range rows[0] {
		header[label] = i
	}
	modelColumn := header[customerExportSummaryHeaderLabel("en", "model")]
	savingsColumn := header[customerExportSummaryHeaderLabel("en", "discount_amount")]
	seen := map[string]string{}
	for _, row := range rows[1:] {
		if name := row[modelColumn]; name != "" {
			seen[name] = row[savingsColumn]
		}
	}
	assert.Equal(t, map[string]string{"known": "0.00020000", "unknown": ""}, seen)
}
