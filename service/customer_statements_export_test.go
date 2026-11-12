package service

import (
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementsExportIncludesAllCustomersFromPinnedMonth(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.BillingStatementMonth{}, &model.BillingStatementVersion{}))
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	end := start.AddDate(0, 1, 0)
	actor := model.User{Id: 9690, Username: "customer-statements-admin", AffCode: "statement9690", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)

	const firstCustomer = 9700
	const customerCount = 22
	var logs []model.Log
	for id := firstCustomer; id < firstCustomer+customerCount; id++ {
		logs = append(logs, model.Log{UserId: id, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 1, Quota: 100, ModelName: "test"})
	}
	require.NoError(t, model.DB.Create(&logs).Error)
	publishBillingExportTestDay(t, start.Unix())
	_, report, err := model.ResolveBillingReport(ctx, start.Unix(), end.Unix()-1, "")
	require.NoError(t, err)
	require.NotEqual(t, "-", report.Version)

	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeCustomerStatements,
		StartTimestamp: start.Unix(), EndTimestamp: end.Unix(), ReportVersion: report.Version, Language: "zh"}
	_, err = SubmitCustomerStatementsExport(ctx, 42, request)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	_, err = SubmitCustomerExportJob(42, 42, request)
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest)
	job, err := SubmitCustomerStatementsExport(ctx, actor.Id, request)
	require.NoError(t, err)
	assert.Zero(t, job.TargetUserId)
	assert.Equal(t, model.CustomerExportJobStatusQueued, job.Status)
	assert.Empty(t, store.uploaded)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, report.Version, filters.BillingReportVersion)
	assert.Equal(t, "USD", filters.Currency)
	require.NotNil(t, filters.BillingReportCoverage)
	assert.Equal(t, 1, filters.BillingReportCoverage.PublishedDays)
	assert.Equal(t, 30, filters.BillingReportCoverage.ExpectedDays)

	require.NoError(t, model.DB.Create(&model.Log{UserId: 9999, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 2, Quota: 500, ModelName: "late"}).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("all-statements-test", common.GetTimestamp()+60, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(ctx, claimed, "all-statements-test", nil)

	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status, completed.Error)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	assert.EqualValues(t, customerCount, artifact.LineCount)
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, customerCount+1)
	assert.Equal(t, "客户 ID", rows[0][1])
	var exported []string
	for _, row := range rows[1:] {
		assert.Equal(t, "2026-09", row[0])
		assert.Equal(t, []string{"1", "30", "0", ""}, row[len(row)-4:])
		exported = append(exported, row[1])
	}
	for id := firstCustomer; id < firstCustomer+customerCount; id++ {
		assert.Contains(t, exported, strconv.Itoa(id))
	}
	_, err = model.GetCustomerExportJobForOwner(job.JobID, 42)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", actor.Id).Update("role", common.RoleCommonUser).Error)
	_, err = model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
}

func TestCustomerStatementsCSVKeepsConfirmedAmountsAndUnknownPrices(t *testing.T) {
	version := 2
	original := "2.00000000"
	items := []model.BillingCustomerStatementListItem{
		{UserId: 1, Username: "confirmed", DisplayName: "=formula",
			MoneyUSD:       &model.BillingStatementListMoney{Gross: "2.50000000", Refund: "1.00000000", Net: "1.50000000", Original: &original},
			BillingVersion: &model.BillingCustomerStatementListVersion{VersionNumber: &version}},
		{UserId: 2, Usage: model.BillingReconciliationUsage{GrossQuota: 100, RefundQuota: 50, NetQuota: 50}},
	}
	filters := model.CustomerExportFilters{StartTimestamp: 1788192000, QuotaPerUnit: 100, Language: "en"}
	_, paths, lines, err := writeCustomerStatementsCSV(context.Background(), t.TempDir(), filters, items)
	require.NoError(t, err)
	require.EqualValues(t, 2, lines)
	rows := readEstimateCsv(t, paths[0])
	require.Len(t, rows, 3)
	assert.Equal(t, "'=formula", rows[1][3])
	assert.Equal(t, "2.00000000", rows[1][5])
	assert.Equal(t, "1.50000000", rows[1][9])
	assert.Equal(t, "2", rows[1][11])
	assert.Empty(t, rows[2][5])
	assert.Empty(t, rows[2][6])
	assert.Equal(t, "0.50000000", rows[2][9])
}
