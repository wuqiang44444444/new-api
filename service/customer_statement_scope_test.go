package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerStatementExportKeepsExistingPartLimits(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, billingExportTimezone())
	month := model.CustomerExportFilters{StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 1, 0).Unix()}
	rows, size := customerStatementExportPartLimits(month, 0)
	assert.Equal(t, 500_000, rows)
	assert.Equal(t, 300_000_000, size)

	for _, scope := range []model.CustomerExportFilters{
		{StartTimestamp: month.StartTimestamp, EndTimestamp: month.StartTimestamp + 86400},
		{StartTimestamp: month.StartTimestamp, EndTimestamp: month.StartTimestamp + 2*86400},
		{StartTimestamp: month.StartTimestamp, EndTimestamp: month.EndTimestamp, TokenName: "prod"},
		{StartTimestamp: month.StartTimestamp, EndTimestamp: month.EndTimestamp, RequestId: "request"},
	} {
		rows, size = customerStatementExportPartLimits(scope, 0)
		assert.Equal(t, 100_000, rows)
		assert.Equal(t, 32*1024*1024, size)
	}
}

func TestCustomerStatementDailyExportUsesNewLimitsAndDoesNotReuseOldFiles(t *testing.T) {
	ctx := context.Background()
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	setupCustomerExportServiceTest(t)
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, billingExportTimezone())
	require.NoError(t, db.Create(&model.Log{
		UserId: 42, RequestId: "daily-export-event", ModelName: "chat",
		CreatedAt: start.Unix() + 1, Type: model.LogTypeConsume, Quota: 125,
	}).Error)
	publishBillingExportTestDay(t, start.Unix())
	request := CustomerExportRequest{
		JobType:        model.CustomerExportJobTypeStatementDetails,
		StartTimestamp: start.Unix(), EndTimestamp: start.AddDate(0, 0, 1).Unix(),
	}
	old, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	legacy, err := old.DecodeFilters()
	require.NoError(t, err)
	legacy.StatementPartLimits = nil
	legacy.FieldVersion = dailyBillingExportFieldVersion
	raw, err := common.Marshal(legacy)
	require.NoError(t, err)
	require.NoError(t, db.Model(old).Update("filters", string(raw)).Error)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("old-daily-format", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(ctx, claimed, claimed.Executor, &customerExportPressureTracker{
		statementPolicy: customerStatementExportPolicy{beforeBatch: func(context.Context) error { return nil }},
	})
	old, err = model.GetCustomerExportJob(old.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, old.Status)

	job, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	require.NotEqual(t, old.JobID, job.JobID)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	require.Equal(t, &model.StatementExportPartLimits{Rows: 1_000_000, Bytes: 500_000_000}, filters.StatementPartLimits)
	rows, bytes := customerStatementExportPartLimits(filters, 0)
	assert.Equal(t, 1_000_000, rows)
	assert.Equal(t, 500_000_000, bytes)
	claimed, err = model.ClaimNextQueuedCustomerExportJob("new-daily-format", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(ctx, claimed, claimed.Executor, &customerExportPressureTracker{
		statementPolicy: customerStatementExportPolicy{beforeBatch: func(context.Context) error { return nil }},
	})
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	require.NotNil(t, finished.DecodeArtifact())
	assert.Equal(t, old.DecodeArtifact().LineCount, finished.DecodeArtifact().LineCount)
	assert.Equal(t, old.DecodeArtifact().StatementTotals, finished.DecodeArtifact().StatementTotals)
	reused, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	assert.Equal(t, job.JobID, reused.JobID)
}

func TestCustomerStatementScopedExportUsesOriginalPublishedDay(t *testing.T) {
	ctx := context.Background()
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	store := setupCustomerExportServiceTest(t)
	startDate := time.Date(2026, 8, 1, 0, 0, 0, 0, billingExportTimezone())
	start, end := startDate.Unix(), startDate.AddDate(0, 1, 0).Unix()
	next := startDate.AddDate(0, 0, 1).Unix()
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 42, TokenId: 3, RequestId: "req", TokenName: "prod", ModelName: "chat", CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 700},
		{UserId: 42, RequestId: "req", TokenName: "prod", ModelName: "chat", CreatedAt: next + 1, Type: model.LogTypeConsume, Quota: 125, IsStream: true, Other: `{"contract_discount":"0.8","contract_id":12,"contract_version":3,"group_ratio":1,"cache_creation_tokens":10,"cache_creation_tokens_5m":3,"cache_creation_tokens_1h":7}`},
		{UserId: 42, RequestId: "task-billing:1:adjustment", TokenName: "prod", ModelName: "chat", CreatedAt: next + 2, Type: model.LogTypeRefund, Quota: 25,
			Other: `{"task_id":"task-for-req","task_billing_event":"adjustment","admin_info":{"task_create_request_id":"req"}}`},
		{UserId: 42, RequestId: "req", TokenName: "prod", ModelName: "chat", CreatedAt: next + 2, Type: model.LogTypeRefund, Quota: 5},
		{UserId: 42, RequestId: "other", TokenName: "prod", ModelName: "chat", CreatedAt: next + 3, Type: model.LogTypeConsume, Quota: 999},
		{UserId: 43, RequestId: "req", TokenName: "prod", ModelName: "chat", CreatedAt: next + 4, Type: model.LogTypeConsume, Quota: 888},
		{UserId: 43, RequestId: "other-account-only", ModelName: "chat", CreatedAt: next + 2*86400 + 1, Type: model.LogTypeConsume, Quota: 444},
	}).Error)
	publishBillingExportTestDay(t, start, next, next+2*86400)
	_, report, err := model.ResolveBillingReport(ctx, start, end-1, "")
	require.NoError(t, err)
	days, err := CustomerStatementDetailExportDays(ctx, 42, 42, start, end, report.Version, "", false)
	require.NoError(t, err)
	assert.Equal(t, []model.CustomerStatementDetailExportDay{
		{Date: "2026-08-02", StartTimestamp: next, EndTimestamp: next + 86400},
		{Date: "2026-08-01", StartTimestamp: start, EndTimestamp: next},
	}, days)
	_, err = CustomerStatementDetailExportDays(ctx, 42, 43, start, end, report.Version, "", false)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: next, EndTimestamp: next + 2*86400,
		ReportVersion: report.Version, RequestId: "req", ModelName: "chat", TokenName: "prod", Language: "zh",
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.NotEqual(t, report.Version, filters.BillingReportVersion)
	require.Equal(t, 1, filters.BillingReportCoverage.PublishedDays)
	require.Equal(t, 2, filters.BillingReportCoverage.ExpectedDays)
	require.NoError(t, db.Create(&model.Log{UserId: 42, RequestId: "req", TokenName: "prod", ModelName: "chat",
		CreatedAt: next + 5, Type: model.LogTypeConsume, Quota: 140}).Error)

	claimed, err := model.ClaimNextQueuedCustomerExportJob("scope-test", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(ctx, claimed, claimed.Executor, &customerExportPressureTracker{
		statementPolicy: customerStatementExportPolicy{partRows: 100, beforeBatch: func(context.Context) error { return nil }},
	})
	finished, err := model.GetCustomerExportJobForOwner(job.JobID, 42)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status, finished.ErrorCode)
	artifact := finished.DecodeArtifact()
	require.Len(t, artifact.Files, 1)
	assert.Contains(t, artifact.Files[0].FileName, "-partial.csv")
	assert.Equal(t, []string{"2026-08-03"}, artifact.MissingDates)
	require.NotNil(t, artifact.StatementTotals)
	assert.EqualValues(t, 95, artifact.StatementTotals.Net)
	assert.NotEmpty(t, artifact.RequestNetAmount)
	require.NotNil(t, finished.ToView().Artifact)
	assert.Equal(t, artifact.RequestNetAmount, finished.ToView().Artifact.RequestNetAmount)
	raw := store.uploaded[artifact.Files[0].ObjectKey]
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte(customerExportCsvBOM)))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 4)
	first := reconciliationTestCells(t, customerStatementDetailColumns, rows[1])
	assert.Equal(t, "是", first["is_stream"])
	assert.Equal(t, "12", first["contract_id"])
	assert.Equal(t, "3", first["write_5m"])
	assert.Equal(t, "7", first["write_1h"])
	refund := reconciliationTestCells(t, customerStatementDetailColumns, rows[2])
	assert.Equal(t, "req", refund["request_id"])
	assert.Equal(t, "退款", refund["event_type"])
	assert.Equal(t, "是", refund["is_stream"])
	unlinked := reconciliationTestCells(t, customerStatementDetailColumns, rows[3])
	assert.Equal(t, "退款", unlinked["event_type"])
	assert.Empty(t, unlinked["is_stream"])

	archive, err := PrepareCustomerExportArchive(finished)
	require.NoError(t, err)
	var destination bytes.Buffer
	require.NoError(t, archive.Write(ctx, &destination))
	bundle, err := zip.NewReader(bytes.NewReader(destination.Bytes()), int64(destination.Len()))
	require.NoError(t, err)
	require.Len(t, bundle.File, 2)
	assert.Equal(t, "report-coverage.csv", bundle.File[1].Name)
	entry, err := bundle.File[1].Open()
	require.NoError(t, err)
	defer entry.Close()
	coverage, err := io.ReadAll(entry)
	require.NoError(t, err)
	assert.Contains(t, string(coverage), "2026-08-03")
	assert.Contains(t, string(coverage), "部分覆盖")
}

func TestCustomerStatementScopedExportReadsConfirmedVersion(t *testing.T) {
	ctx := context.Background()
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	setupCustomerExportServiceTest(t)
	startDate := time.Date(2026, 8, 1, 0, 0, 0, 0, billingExportTimezone())
	start := startDate.Unix()
	version := model.BillingStatementVersion{
		UserId: 42, DraftPublicId: "scoped-confirmed", PeriodStart: start, PeriodEndExclusive: startDate.AddDate(0, 1, 0).Unix(),
		Currency: "USD", QuotaPerUnit: 100, CurrencyRate: 1, Status: model.BillingStatementVersionConfirmed,
	}
	require.NoError(t, db.Create(&version).Error)
	for i, request := range []string{"matching", "other", "other-too"} {
		createdAt := start + 100
		if i > 0 {
			createdAt += int64(i) * 86400
		}
		facts, err := common.Marshal(model.CustomerExportRow{RecordTime: createdAt, RequestId: request,
			EventType: "consume", Quota: 125, CustomerModel: "chat", TokenName: "prod"})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.BillingStatementVersionLine{
			VersionId: version.ID, Sequence: int64(2 - i), RequestId: request, TokenName: "prod", CustomerModel: "chat",
			CreatedAt: createdAt, LogType: model.LogTypeConsume, Quota: 125, Facts: string(facts),
		}).Error)
	}
	days, err := CustomerStatementDetailExportDays(ctx, 42, 42, start, version.PeriodEndExclusive, "", version.DraftPublicId, false)
	require.NoError(t, err)
	require.Len(t, days, 3)
	assert.Equal(t, "2026-08-03", days[0].Date)
	assert.Equal(t, "2026-08-01", days[2].Date)
	_, err = CustomerStatementDetailExportDays(ctx, 43, 43, start, version.PeriodEndExclusive, "", version.DraftPublicId, false)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	require.NoError(t, db.Model(&version).Update("status", model.BillingStatementVersionPending).Error)
	_, err = CustomerStatementDetailExportDays(ctx, 42, 42, start, version.PeriodEndExclusive, "", version.DraftPublicId, false)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	require.NoError(t, db.Model(&version).Update("status", model.BillingStatementVersionConfirmed).Error)
	request := CustomerExportRequest{JobType: model.CustomerExportJobTypeStatementDetails, StatementVersion: version.DraftPublicId,
		StartTimestamp: start, EndTimestamp: start + 86400, ModelName: "chat", RequestId: "matching"}
	_, err = SubmitCustomerExportJob(43, 43, request)
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	job, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	assert.Empty(t, filters.BillingReportVersion)
	assert.Equal(t, float64(100), filters.QuotaPerUnit)
	page, err := model.ReadCustomerStatementExportEvents(ctx, db, 42, filters, 0, 2)
	require.NoError(t, err)
	require.Len(t, page.Events, 1)
	assert.True(t, page.Done)
	assert.Equal(t, 1, page.Scanned)
	assert.Equal(t, "matching", page.Events[0].Row.RequestId)
	assert.EqualValues(t, 125, page.Events[0].Charge)

	for _, line := range []struct {
		sequence  int64
		createdAt int64
		request   string
	}{
		{3, start + 50, "earliest"},
		{4, start + 100, "same-time"},
		{5, start + 86400, "next-day-midnight"},
	} {
		facts, err := common.Marshal(model.CustomerExportRow{
			RecordTime: line.createdAt, RequestId: line.request, EventType: "consume", Quota: 125,
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.BillingStatementVersionLine{
			VersionId: version.ID, Sequence: line.sequence, CreatedAt: line.createdAt, RequestId: line.request,
			LogType: model.LogTypeConsume, Quota: 125, Facts: string(facts),
		}).Error)
	}
	filters.RequestId, filters.ModelName = "", ""
	var cursor int64
	for i, request := range []string{"earliest", "matching", "same-time"} {
		page, err := model.ReadCustomerStatementExportEvents(ctx, db, 42, filters, cursor, 1)
		require.NoError(t, err)
		require.Len(t, page.Events, 1)
		assert.Equal(t, request, page.Events[0].Row.RequestId)
		assert.Equal(t, i == 2, page.Done)
		assert.Equal(t, 1, page.Scanned)
		cursor = page.Events[0].SourceID
	}
}
