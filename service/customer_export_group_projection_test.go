package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportDetailsPreserveBillingGroupIdentity(t *testing.T) {
	ratio := 0.5
	writer, err := newCustomerExportCsvWriter(t.TempDir(), "export", customerExportCsvShardBytes)
	require.NoError(t, err)
	writer.useStatementDetails("en")
	scope := customerExportScopeColumns{Language: "en", QuotaPerUnit: 500000, Currency: "USD", CurrencyRate: 1}
	for _, row := range []model.CustomerExportRow{
		{GroupName: "historical-a", GroupRatioSource: "group", GroupRatio: &ratio},
		{GroupName: "=historical-b", GroupRatioSource: "user_exclusive", GroupRatio: &ratio},
	} {
		require.NoError(t, writer.AppendRow(row, scope))
	}
	_, paths, _, _, err := writer.Finish()
	require.NoError(t, err)
	records := readEstimateCsv(t, paths[0])
	header := map[string]int{}
	for i, name := range records[0] {
		header[name] = i
	}
	require.Contains(t, header, "Billing group")
	require.Contains(t, header, "Group ratio source")
	seen := map[string]string{}
	for _, record := range records[1:] {
		seen[record[header["Billing group"]]] = record[header["Group ratio source"]]
	}
	assert.Equal(t, map[string]string{"historical-a": "Group", "'=historical-b": "Customer-specific group factor"}, seen)
}
