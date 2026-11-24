package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementSummaryContainsOnlyAdditiveModelRows(t *testing.T) {
	original, discount := int64(200), int64(100)
	usage := model.BillingReconciliationUsage{Requests: 2, GrossQuota: 150, RefundQuota: 50, NetQuota: 100}
	statement := model.BillingCustomerStatement{
		Summary: usage, OriginalQuota: &original,
		Groups: []model.BillingReconciliationGroupSummary{{Id: 4, Name: "=key", Usage: usage,
			Models: []model.BillingReconciliationModelSummary{{ModelName: "model", BillingMode: "token", Usage: usage, OriginalQuota: &original, DiscountQuota: &discount}}}},
		DiscountCombinations: []model.BillingDiscountCombination{{GroupId: 4, Usage: usage}},
	}
	for _, language := range []string{"en", "zh"} {
		t.Run(language, func(t *testing.T) {
			scope := customerExportScopeColumns{QuotaPerUnit: 100, Currency: "USD", CurrencyRate: 1,
				ReportCoverage: &model.BillingExportCoverage{PublishedDays: 10, ExpectedDays: 30, IncompleteDays: 1}}
			_, paths, count, err := writeCustomerExportSummaryCsv(t.TempDir(), scope, language, statement)
			require.NoError(t, err)
			assert.EqualValues(t, 1, count)
			rows := readEstimateCsv(t, paths[0])
			require.Len(t, rows, 2)
			values := reconciliationTestCells(t, customerSummaryColumns, rows[1])
			assert.Equal(t, "'=key", values["api_key_name"])
			assert.Equal(t, "2", values["requests"])
			for key, value := range map[string]string{"net_amount": "1.00000000", "original_amount": "2.00000000", "discount_amount": "1.00000000", "gross_amount": "1.50000000", "refund_amount": "0.50000000"} {
				assert.Equal(t, value, values[key], key)
			}
			assert.Contains(t, values["notes"], "10/30")
			assert.NotContains(t, values, "row_type")
			assert.NotContains(t, values, "job_id")
		})
	}
}

func TestCustomerStatementDetailsExposeCallsAndSignedAmounts(t *testing.T) {
	for _, language := range []string{"en", "zh"} {
		t.Run(language, func(t *testing.T) {
			writer, err := newCustomerExportCsvWriter(t.TempDir(), "export", 1)
			require.NoError(t, err)
			writer.useStatementDetails(language, "CNY")
			scope := customerExportScopeColumns{Language: language, QuotaPerUnit: 100, Currency: "CNY", CurrencyRate: 7}
			ratio := 0.5
			rows := []model.CustomerExportRow{
				{RequestId: "req-1", RecordTime: 1789488000, TokenId: 4, TokenName: "=key", CustomerModel: "model", EventType: "consume", RequestCount: 1, Quota: 100, RecordedOriginal: "200", ContractApplicable: "unrecorded", GroupRatio: &ratio, FinalRatio: &ratio, InputTokens: 1000,
					BillingLineItems: `[{"label":"Input","quantity":1000,"unit":"token","unit_price_usd":2,"unit_price_quantity":1000000,"subtotal_usd":0.002}]`, ExplanationStatus: "available"},
				{RequestId: "req-1", EventType: "refund", Quota: 50, RecordedOriginal: "-100", ExplanationStatus: "not_applicable"},
				{RequestId: "req-unknown", EventType: "consume", Quota: 10, OriginalEstimate: "20", InputTokensUnavailable: true, EstimateReasons: []string{model.BillingEstimateMissingGroup}},
				{RequestId: "req-free", EventType: "consume", RecordedOriginal: "0"},
			}
			for _, row := range rows {
				require.NoError(t, writer.AppendRow(row, scope))
			}
			files, paths, count, _, err := writer.Finish()
			require.NoError(t, err)
			require.Len(t, files, 4)
			assert.EqualValues(t, 4, count)
			assert.Equal(t, "statement-details.csv", files[0].FileName)
			assert.Equal(t, "statement-details.part2.csv", files[1].FileName)
			var records []map[string]string
			for _, path := range paths {
				csvRows := readEstimateCsv(t, path)
				require.Len(t, csvRows, 2)
				assert.Equal(t, writer.header, csvRows[0])
				values := map[string]string{}
				for i, column := range customerStatementDetailColumns {
					values[column.key] = csvRows[1][i]
				}
				records = append(records, values)
			}
			assert.Equal(t, "req-1", records[0]["request_id"])
			assert.Equal(t, formatExportTimestamp(1789488000), records[0]["record_time"])
			assert.Equal(t, "'=key", records[0]["token_name"])
			assert.Equal(t, "1000", records[0]["input_tokens"])
			assert.Equal(t, "0.5", records[0]["final_ratio"])
			assert.Equal(t, "7.00000000", records[0]["net_amount"])
			assert.Equal(t, "14.00000000", records[0]["original_amount"])
			assert.Equal(t, "req-1", records[1]["request_id"])
			assert.Equal(t, "0", records[1]["request_count"])
			assert.Equal(t, "-3.50000000", records[1]["net_amount"])
			assert.Empty(t, records[2]["original_amount"])
			assert.Empty(t, records[2]["input_tokens"])
			assert.Equal(t, "0.00000000", records[3]["original_amount"])
			assert.NotContains(t, records[0], "billing_line_items_usd")
			if language == "zh" {
				assert.Equal(t, "入账时间", writer.header[0])
				assert.Equal(t, "扣款", records[0]["event_type"])
				assert.Equal(t, "退款", records[1]["event_type"])
			} else {
				assert.Equal(t, "Charge", records[0]["event_type"])
				assert.Equal(t, "Refund", records[1]["event_type"])
			}
		})
	}
}
