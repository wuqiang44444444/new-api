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
		StartTimestamp: start.Unix(), EndTimestamp: end.Unix(), ReportVersion: report.Version, Language: "en"}
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
	assert.Equal(t, dailyBillingExportFieldVersion, filters.FieldVersion)
	assert.Equal(t, "zh", filters.Language)
	assert.Equal(t, report.Version, filters.BillingReportVersion)
	assert.Equal(t, "USD", filters.Currency)
	require.NotNil(t, filters.BillingReportCoverage)
	assert.Equal(t, 1, filters.BillingReportCoverage.PublishedDays)
	assert.Equal(t, 30, filters.BillingReportCoverage.ExpectedDays)

	// Simulate a month export queued by the previous release.
	legacy := filters
	legacy.FieldVersion = 26
	legacy.Language = "en"
	encoded, err := common.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(job).Update("filters", string(encoded)).Error)
	require.NoError(t, model.DB.Create(&model.Log{UserId: 9999, Type: model.LogTypeConsume, CreatedAt: start.Unix() + 2, Quota: 500, ModelName: "late"}).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("all-statements-test", common.GetTimestamp()+60, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(ctx, claimed, "all-statements-test", nil)

	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status, completed.Error)
	upgraded, err := completed.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, filters, upgraded)
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
		assert.Contains(t, row[10], "部分覆盖，已发布 1/30 天")
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
	assert.Equal(t, "'=formula", rows[1][2])
	assert.Equal(t, "2.00000000", rows[1][6])
	assert.Equal(t, "1.50000000", rows[1][3])
	assert.Equal(t, "2", rows[1][9])
	assert.Empty(t, rows[2][6])
	assert.Empty(t, rows[2][7])
	assert.Equal(t, "0.50000000", rows[2][3])
}

func TestCustomerStatementsExportDayIgnoresConfirmedMonthAndPinsDay(t *testing.T) {
	truncate(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.BillingStatementMonth{}, &model.BillingStatementVersion{}))
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	end := start.AddDate(0, 0, 1)
	actor := model.User{Id: 9691, Username: "daily-statements-admin", AffCode: "statement9691", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(&actor).Error)
	const customer = 9700
	confirmed := model.BillingStatementVersion{UserId: customer, PeriodStart: start.Unix(),
		PeriodEndExclusive: start.AddDate(0, 1, 0).Unix(), DraftPublicId: "confirmed-month-daily-export",
		Status: model.BillingStatementVersionConfirmed}
	require.NoError(t, model.DB.Create(&confirmed).Error)
	require.NoError(t, model.DB.Create(&model.BillingStatementMonth{UserId: customer, PeriodStart: start.Unix(), CurrentVersionId: &confirmed.ID}).Error)
	require.NoError(t, model.DB.Create(&[]model.Log{
		{UserId: customer, Type: model.LogTypeConsume, CreatedAt: start.Unix(), Quota: 100, ModelName: "billed"},
		{UserId: customer, Type: model.LogTypeConsume, CreatedAt: end.Unix() - 1, Quota: 200, ModelName: "billed"},
		{UserId: customer + 1, Type: model.LogTypeConsume, CreatedAt: end.Unix(), Quota: 300, ModelName: "next-day"},
	}).Error)
	publishBillingExportTestDay(t, start.Unix(), end.Unix())
	_, report, err := model.ResolveBillingReport(ctx, start.Unix(), end.Unix()-1, "")
	require.NoError(t, err)
	require.Equal(t, 1, report.PublishedDays)

	expectedIncomplete := false
	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeCustomerStatements,
		BillingDate: "2026-09-01", ReportVersion: report.Version, Language: "zh", ExpectedIncomplete: &expectedIncomplete}
	_, err = SubmitCustomerStatementsExport(ctx, 42, request)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)

	wrongDay := request
	wrongDay.BillingDate = "2026-09-02"
	_, err = SubmitCustomerStatementsExport(ctx, actor.Id, wrongDay)
	require.Error(t, err)
	ambiguous := request
	ambiguous.StartTimestamp, ambiguous.EndTimestamp = start.Unix(), end.Unix()
	_, err = SubmitCustomerStatementsExport(ctx, actor.Id, ambiguous)
	require.ErrorIs(t, err, ErrCustomerExportInvalidRequest)

	job, err := SubmitCustomerStatementsExport(ctx, actor.Id, request)
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Equal(t, start.Unix(), filters.StartTimestamp)
	assert.Equal(t, end.Unix(), filters.EndTimestamp)
	assert.Equal(t, report.Version, filters.BillingReportVersion)
	assert.Greater(t, filters.FieldVersion, 26, "the monthly runner must reject daily jobs")

	require.NoError(t, model.DB.Create(&model.Log{UserId: customer + 2, Type: model.LogTypeConsume,
		CreatedAt: start.Unix() + 5, Quota: 400, ModelName: "late"}).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("daily-test", common.GetTimestamp()+60, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(ctx, claimed, "daily-test", nil)

	completed, err := model.GetCustomerExportJobForOwner(job.JobID, actor.Id)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, completed.Status, completed.Error)
	artifact := completed.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	assert.Contains(t, artifact.Files[0].FileName, "2026-09-01")
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[artifact.Files[0].ObjectKey]), customerExportCsvBOM))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "账单日期（上海时区）", rows[0][0])
	assert.Equal(t, "账单版本", rows[0][9])
	assert.Equal(t, []string{"2026-09-01", "9700"}, rows[1][:2])
	assert.Equal(t, "2", rows[1][8])
	assert.Equal(t, report.Version, rows[1][9])
	assert.Len(t, rows[1], 11)

	t.Run("monthly format cannot publish a daily job", func(t *testing.T) {
		filters.FieldVersion = 26
		oldFormat, _, err := model.CreateCustomerExportJob(actor.Id, 0, model.CustomerExportJobTypeCustomerStatements, filters)
		require.NoError(t, err)
		claimed, err := model.ClaimNextQueuedCustomerExportJob("legacy-format-test", common.GetTimestamp()+60, 1800)
		require.NoError(t, err)
		require.NotNil(t, claimed)
		require.Equal(t, oldFormat.JobID, claimed.JobID)
		runCustomerExportJob(ctx, claimed, "legacy-format-test", nil)

		rejected, err := model.GetCustomerExportJobForOwner(oldFormat.JobID, actor.Id)
		require.NoError(t, err)
		assert.Equal(t, model.CustomerExportJobStatusFailed, rejected.Status)
		assert.Equal(t, "format_changed", rejected.ErrorCode)
		assert.Nil(t, rejected.DecodeArtifact())
	})
}

func TestCustomerStatementsExportDayCoverageChangesAndFrozenRegeneration(t *testing.T) {
	truncate(t)
	setupCustomerExportServiceTest(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 10, 0, 0, 0, 0, billingExportTimezone())
	end := start.AddDate(0, 0, 1)
	admins := []model.User{
		{Id: 9692, Username: "coverage-admin", AffCode: "statement9692", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
		{Id: 9693, Username: "other-coverage-admin", AffCode: "statement9693", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
	}
	require.NoError(t, model.DB.Create(&admins).Error)
	require.NoError(t, model.DB.Create(&model.Log{UserId: 9700, Type: model.LogTypeConsume,
		CreatedAt: start.Unix(), Quota: 400, ModelName: "billed"}).Error)
	publishBillingExportTestDay(t, start.Unix())
	_, report, err := model.ResolveBillingReport(ctx, start.Unix(), end.Unix()-1, "")
	require.NoError(t, err)
	require.Equal(t, 1, report.PublishedDays)
	require.Zero(t, report.IncompleteDays)
	complete := false
	request := CustomerExportRequest{BillingDate: start.Format(time.DateOnly), ReportVersion: report.Version,
		Language: "zh", ExpectedIncomplete: &complete}
	first, err := SubmitCustomerStatementsExport(ctx, admins[0].Id, request)
	require.NoError(t, err)

	require.NoError(t, model.DB.Table("daily_report_retries").Create(map[string]any{
		"start": start.Unix(), "kind": model.DailyReportBilling, "coverage_issue": true,
		"release_version": model.CustomerOriginalReplayVersion,
	}).Error)

	duplicate, err := SubmitCustomerStatementsExport(ctx, admins[0].Id, request)
	require.NoError(t, err)
	assert.Equal(t, first.JobID, duplicate.JobID)
	assert.Equal(t, 0, duplicate.ToView().Filters.BillingReportCoverage.IncompleteDays)
	_, err = model.CancelCustomerExportJob(first.JobID, admins[0].Id)
	require.NoError(t, err)

	rejected, err := SubmitCustomerStatementsExport(ctx, admins[0].Id, request)
	require.ErrorIs(t, err, model.ErrCustomerStatementExportCoverageChanged)
	assert.Nil(t, rejected)

	incomplete := true
	request.ExpectedIncomplete = &incomplete
	partial, err := SubmitCustomerStatementsExport(ctx, admins[0].Id, request)
	require.NoError(t, err)
	assert.Equal(t, 1, partial.ToView().Filters.BillingReportCoverage.IncompleteDays)
	_, err = model.CancelCustomerExportJob(partial.JobID, admins[0].Id)
	require.NoError(t, err)
	require.NoError(t, model.DB.Exec("DELETE FROM daily_report_retries WHERE start = ? AND kind = ?", start.Unix(), model.DailyReportBilling).Error)

	_, err = SubmitCustomerStatementsExport(ctx, admins[1].Id, CustomerExportRequest{SourceJobID: partial.JobID})
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	regenerated, err := SubmitCustomerStatementsExport(ctx, admins[0].Id, CustomerExportRequest{SourceJobID: partial.JobID})
	require.NoError(t, err)
	assert.Equal(t, partial.ToView().Filters, regenerated.ToView().Filters)
}
