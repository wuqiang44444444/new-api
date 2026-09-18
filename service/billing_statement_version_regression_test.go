package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementCSVCompleteAcrossBatches(t *testing.T) {
	for _, count := range []int{101, 1001, 100001} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			db := setupVersionServiceTestDB(t)
			t.Setenv("TMPDIR", t.TempDir())
			v := &model.BillingStatementVersion{DraftPublicId: fmt.Sprintf("bsv_rows_%d", count), QuotaPerUnit: 1000000, Currency: "USD", CurrencyRate: 1}
			require.NoError(t, db.Create(v).Error)
			gross := int64(count-1) * 17
			refund := int64(7)
			raw, err := common.Marshal(model.BillingStatementIntegrity{Rows: int64(count), Gross: gross, Refund: refund})
			require.NoError(t, err)
			v.Integrity = string(raw)
			for start := 0; start < count; start += 500 {
				batch := make([]model.BillingStatementVersionLine, 0, 500)
				for n := start; n < min(count, start+500); n++ {
					facts := model.CustomerExportRow{RequestId: fmt.Sprintf("req-%d", n), EventType: "consume", Quota: 17, CountsTowardBill: true, GroupName: "group", ContractApplicable: "yes", ContractName: "contract"}
					logType := model.LogTypeConsume
					if n == count-1 {
						facts.EventType = "refund"
						facts.Quota = refund
						logType = model.LogTypeRefund
					}
					data, err := common.Marshal(facts)
					require.NoError(t, err)
					batch = append(batch, model.BillingStatementVersionLine{VersionId: v.ID, Sequence: int64(n), SourceLogId: int64(n + 1), LogType: logType, Quota: facts.Quota, Facts: string(data)})
				}
				require.NoError(t, db.Create(&batch).Error)
			}
			file, err := writeBillingStatementVersionDetailCsv(context.Background(), v)
			require.NoError(t, err)
			require.NotNil(t, file)
			assert.EqualValues(t, count, file.lineCount)
			input, err := os.Open(file.path)
			require.NoError(t, err)
			defer input.Close()
			reader := csv.NewReader(input)
			header, err := reader.Read()
			require.NoError(t, err)
			columns := map[string]int{}
			for i, name := range header {
				columns[name] = i
			}
			require.Contains(t, columns, "Request ID")
			require.Contains(t, columns, "Net amount (refunds negative)")
			assert.Equal(t, "statement-details.csv", file.fileName)
			total := decimal.Zero
			rows := 0
			for {
				record, err := reader.Read()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.Equal(t, fmt.Sprintf("req-%d", rows), record[columns["Request ID"]])
				require.Equal(t, "contract", record[columns["Contract"]])
				amount, err := decimal.NewFromString(record[columns["Net amount (refunds negative)"]])
				require.NoError(t, err)
				total = total.Add(amount)
				rows++
			}
			assert.Equal(t, count, rows)
			assert.True(t, decimal.NewFromInt(gross-refund).Div(decimal.NewFromInt(1000000)).Equal(total), "CSV net amount must match the frozen detail")
			// 汇总证明不符时禁止输出成功文件。
			v.Integrity = `{"rows":0,"gross":0,"refund":0}`
			_, err = writeBillingStatementVersionDetailCsv(context.Background(), v)
			assert.Error(t, err)
		})
	}
}

func TestBillingStatementDownloadFilenameIsSigned(t *testing.T) {
	store := newTestS3Store(t)
	signed, _, err := store.ExportPresignURL("billing/statements/draft/file.csv", time.Minute, "2026-08-v2-detail.csv")
	require.NoError(t, err)
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	assert.Equal(t, `attachment; filename=2026-08-v2-detail.csv`, parsed.Query().Get("response-content-disposition"))
	assert.NotEmpty(t, parsed.Query().Get("X-Amz-Signature"))
}
