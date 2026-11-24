package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerExportDetailsKeepsEffectiveMultiplierWithoutChannelIdentity(t *testing.T) {
	ratio := 0.5
	writer, err := newCustomerExportCsvWriter(t.TempDir(), "export", customerExportCsvShardBytes)
	require.NoError(t, err)
	writer.useStatementDetails("en", "USD")
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
	assert.NotContains(t, records[0], "Billing group")
	assert.NotContains(t, records[0], "Channel ID")
	for _, record := range records[1:] {
		cells := reconciliationTestCells(t, customerStatementDetailColumns, record)
		assert.Equal(t, "0.5", cells["group_ratio"])
	}
}
