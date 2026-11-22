package service

import (
	"context"
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interruptedStatementExportStore struct {
	*stubExportStore
	interrupt context.CancelFunc
}

func (s *interruptedStatementExportStore) ExportPutFile(ctx context.Context, key, mediaType, path string, size int64) error {
	err := s.stubExportStore.ExportPutFile(ctx, key, mediaType, path, size)
	s.interrupt()
	return err
}

type statementExportUploadTransport func(*http.Request) (*http.Response, error)

func (f statementExportUploadTransport) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStatementDetailsUploadUsesRemainingJobBudget(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	const start = int64(1785513600)
	require.NoError(t, db.Create(&model.Log{
		UserId: 42, CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 25, RequestId: "first",
	}).Error)
	publishBillingExportTestDay(t, start)
	config := system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendAzureBlob, Endpoint: "https://fixture.invalid", Bucket: "archives", AccountName: "fixture",
	}
	store, err := NewAzureBlobArtifactStore(config, "Zml4dHVyZQ==")
	require.NoError(t, err)
	azure := store.(*azureBlobArtifactStore)
	customerExportStoreOverride = azure

	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 31*86400,
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	ctx, _, err := model.ResolveBillingReport(t.Context(), start, filters.EndTimestamp-1, filters.BillingReportVersion)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("statement-upload-deadline", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	jobCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	jobDeadline, _ := jobCtx.Deadline()
	var uploadDeadline time.Time
	azure.client, err = azblob.NewClientWithSharedKeyCredential(config.Endpoint, azure.credential, &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: statementExportUploadTransport(func(req *http.Request) (*http.Response, error) {
			uploadDeadline, _ = req.Context().Deadline()
			return &http.Response{StatusCode: http.StatusCreated, Body: http.NoBody, Header: http.Header{"ETag": {`"fixture"`}}}, nil
		})},
	})
	require.NoError(t, err)
	policy := newCustomerStatementExportPolicy()
	policy.beforeBatch = func(context.Context) error { return nil }
	policy.currentRate = nil
	artifact, err := executeCustomerStatementExport(jobCtx, claimed, filters, t.TempDir(), policy)
	require.NoError(t, err)
	assert.Equal(t, jobDeadline, uploadDeadline)
	assert.EqualValues(t, 1, artifact.LineCount)
}

func TestStatementDetailsKeepsShardsAndAmountsAcrossPressureAndLeaseLoss(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	store := setupCustomerExportServiceTest(t)
	const start = int64(1785513600)
	for i, quota := range []int{100, 20, 50} {
		logType := model.LogTypeConsume
		if i == 1 {
			logType = model.LogTypeRefund
		}
		require.NoError(t, db.Create(&model.Log{UserId: 42, TokenId: 7, ModelName: "test-model",
			CreatedAt: start + int64(i+1), Type: logType, Quota: quota, RequestId: []string{"first", "refund", "third"}[i],
			Other: `{"group_ratio":1}`}).Error)
	}
	publishBillingExportTestDay(t, start)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 31*86400, Language: "en",
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	ctx, _, err := model.ResolveBillingReport(context.Background(), start, filters.EndTimestamp-1, filters.BillingReportVersion)
	require.NoError(t, err)

	claim := func(worker string) *model.CustomerExportJob {
		t.Helper()
		claimed, err := model.ClaimNextQueuedCustomerExportJob(worker, common.GetTimestamp()+120, 1800)
		require.NoError(t, err)
		require.NotNil(t, claimed)
		require.Equal(t, job.JobID, claimed.JobID)
		return claimed
	}
	dir := t.TempDir()
	policy := customerStatementExportPolicy{partRows: 1, beforeBatch: func(context.Context) error {
		if len(store.uploaded) == 1 {
			return errCustomerStatementExportPressure
		}
		return nil
	}}
	claimed := claim("statement-first")
	_, err = executeCustomerStatementExport(ctx, claimed, filters, dir, policy)
	require.ErrorIs(t, err, errCustomerStatementExportPressure)
	require.NoError(t, model.PauseCustomerStatementExport(ctx, claimed.JobID, claimed.Executor, common.GetTimestamp(), false, true))
	paused, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.True(t, paused.DecodeProgress().WaitingE)
	assert.EqualValues(t, 1, paused.DecodeProgress().Written)
	assert.Nil(t, paused.ToView().Artifact)

	claimed = claim("statement-interrupted")
	interruptedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	customerExportStoreOverride = &interruptedStatementExportStore{store, cancel}
	policy.beforeBatch = func(context.Context) error { return nil }
	_, err = executeCustomerStatementExport(interruptedCtx, claimed, filters, dir, policy)
	require.ErrorIs(t, err, context.Canceled)
	interrupted, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.True(t, interrupted.DecodeArtifact().Checkpoint.PendingFile)
	assert.EqualValues(t, 1, interrupted.DecodeProgress().Written)
	assert.Len(t, store.uploaded, 2)
	customerExportStoreOverride = store
	require.NoError(t, db.Model(&model.CustomerExportJob{}).Where("job_id = ?", job.JobID).Update("lease_until", common.GetTimestamp()-1).Error)
	recovered, err := model.RecoverInterruptedCustomerExportJobs(common.GetTimestamp())
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)
	claimed = claim("statement-final")
	policy.beforeBatch = func(context.Context) error { return nil }
	policy.partRows = statementExportPartRows
	artifact, err := executeCustomerStatementExport(ctx, claimed, filters, dir, policy)
	require.NoError(t, err)
	require.NoError(t, model.FinishCustomerExportJob(job.JobID, claimed.Executor, model.CustomerExportJobStatusSucceeded, "", "", artifact))

	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	artifact = finished.DecodeArtifact()
	require.Len(t, artifact.Files, 3)
	assert.EqualValues(t, 3, artifact.LineCount)
	assert.Equal(t, &model.CustomerStatementExportTotals{Charges: 150, Refunds: 20, Net: 130}, artifact.StatementTotals)
	assert.Nil(t, artifact.Checkpoint)
	assert.Contains(t, artifact.MissingDates, "2026-08-02")
	var requestIDs, amounts []string
	for _, file := range artifact.Files {
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(store.uploaded[file.ObjectKey]), customerExportCsvBOM))).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 2)
		for i, label := range rows[0] {
			switch label {
			case "Request ID":
				requestIDs = append(requestIDs, rows[1][i])
			case "Net amount (refunds negative)":
				amounts = append(amounts, rows[1][i])
			}
		}
	}
	assert.Equal(t, []string{"first", "refund", "third"}, requestIDs)
	expected := customerExportScopeColumns{Currency: filters.Currency, CurrencyRate: filters.CurrencyRate, QuotaPerUnit: filters.QuotaPerUnit}
	assert.Equal(t, []string{exportCurrencyAmount("100", expected), exportCurrencyAmount("-20", expected), exportCurrencyAmount("50", expected)}, amounts)
}

func TestStatementDetailsWaitsWhenFrozenVersionIsNoLongerReadable(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	store := setupCustomerExportServiceTest(t)
	const start = int64(1785513600)
	require.NoError(t, db.Create(&model.Log{UserId: 42, TokenId: 7, ModelName: "test-model",
		CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 10}).Error)
	publishBillingExportTestDay(t, start)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 31*86400, Language: "en",
	})
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("statement-missing-version", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	pressure := &customerExportPressureTracker{statementPolicy: customerStatementExportPolicy{
		partRows: 10,
		beforeBatch: func(ctx context.Context) error {
			// The job has resolved its pinned generation. Simulate losing that
			// generation before its read pool validates the source.
			result := db.WithContext(ctx).Table("billing_report_generations").Where("published = ?", true).Update("published", false)
			require.NoError(t, result.Error)
			require.EqualValues(t, 1, result.RowsAffected)
			return nil
		},
	}}
	assert.True(t, runCustomerExportJob(context.Background(), claimed, claimed.Executor, pressure))
	paused, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Equal(t, model.CustomerExportJobStatusQueued, paused.Status)
	assert.True(t, paused.DecodeProgress().WaitingE)
	assert.Empty(t, store.uploaded)
}

func TestStatementDetailsCommitsPartialShardBeforeYielding(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	store := setupCustomerExportServiceTest(t)
	const start = int64(1785513600)
	require.NoError(t, db.Create(&[]model.Log{
		{UserId: 42, CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 25, RequestId: "first"},
		{UserId: 42, CreatedAt: start + 2, Type: model.LogTypeConsume, Quota: 75, RequestId: "second"},
	}).Error)
	publishBillingExportTestDay(t, start)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 31*86400,
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	ctx, _, err := model.ResolveBillingReport(t.Context(), start, filters.EndTimestamp-1, filters.BillingReportVersion)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("statement-yield", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)

	reportDB, closeSource, err := model.OpenCustomerStatementExportSource()
	require.NoError(t, err)
	defer closeSource()
	readCtx, finishReading := context.WithCancelCause(ctx)
	defer finishReading(nil)
	policy := newCustomerStatementExportPolicy()
	policy.beforeBatch = func(context.Context) error { return nil }
	rateCalls := 0
	policy.currentRate = func() int {
		rateCalls++
		if rateCalls == 1 {
			return 8 // Read one event in the first batch.
		}
		finishReading(errCustomerStatementExportYield)
		return statementExportMaxRowsPerSecond
	}
	artifact := &model.CustomerExportArtifact{
		StoreIdentity: store.ExportIdentity(), GeneratedAt: common.GetTimestamp(),
		Checkpoint: &model.CustomerStatementExportCheckpoint{}, StatementTotals: &model.CustomerStatementExportTotals{},
	}
	scope := customerExportScopeColumns{Language: filters.Language, QuotaPerUnit: filters.QuotaPerUnit, CurrencyRate: filters.CurrencyRate}
	dir := t.TempDir()
	done, err := writeCustomerStatementExportPart(ctx, readCtx, reportDB, store, claimed, filters, artifact, nil, scope, dir, policy)
	require.NoError(t, err)
	assert.False(t, done)
	stored, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stored.DecodeProgress().Written)
	require.Len(t, artifact.Files, 1)
	assert.Contains(t, string(store.uploaded[artifact.Files[0].ObjectKey]), "first")

	policy.currentRate = nil
	finished, err := executeCustomerStatementExport(ctx, stored, filters, dir, policy)
	require.NoError(t, err)
	require.Len(t, finished.Files, 2)
	assert.EqualValues(t, 2, finished.LineCount)
	assert.EqualValues(t, 100, finished.StatementTotals.Net)
	assert.Contains(t, string(store.uploaded[finished.Files[1].ObjectKey]), "second")
}

func TestStatementDetailsResumesAfterCommittingAnEmptyCandidateWindow(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	store := setupCustomerExportServiceTest(t)
	const start = int64(1785513600)
	rows := []model.Log{
		{UserId: 42, CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 25, RequestId: "unmatched"},
		{UserId: 42, CreatedAt: start + 2, Type: model.LogTypeConsume, Quota: 75, RequestId: "selected"},
	}
	require.NoError(t, db.Create(&rows).Error)
	publishBillingExportTestDay(t, start)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 31*86400,
		RequestId: "selected",
	})
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	ctx, _, err := model.ResolveBillingReport(t.Context(), start, filters.EndTimestamp-1, filters.BillingReportVersion)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("sparse-first", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	policy := customerStatementExportPolicy{partRows: 1, beforeBatch: func(context.Context) error {
		stored, err := model.GetCustomerExportJob(job.JobID)
		if err != nil {
			return err
		}
		if artifact := stored.DecodeArtifact(); artifact != nil && artifact.Checkpoint.Cursor > 0 {
			return errCustomerStatementExportPressure
		}
		return nil
	}}
	workDir := t.TempDir()
	_, err = executeCustomerStatementExport(ctx, claimed, filters, workDir, policy)
	require.ErrorIs(t, err, errCustomerStatementExportPressure)
	require.NoError(t, model.PauseCustomerStatementExport(ctx, job.JobID, claimed.Executor, common.GetTimestamp(), false, true))
	paused, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	assert.Empty(t, store.uploaded)
	assert.Zero(t, paused.DecodeProgress().Written)
	assert.Positive(t, paused.DecodeArtifact().Checkpoint.Cursor)
	claimed, err = model.ClaimNextQueuedCustomerExportJob("sparse-resumed", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	policy.beforeBatch = func(context.Context) error { return nil }
	artifact, err := executeCustomerStatementExport(ctx, claimed, filters, workDir, policy)
	require.NoError(t, err)
	require.Len(t, artifact.Files, 1)
	assert.EqualValues(t, 1, artifact.LineCount)
	assert.Equal(t, &model.CustomerStatementExportTotals{Charges: 75, Net: 75}, artifact.StatementTotals)
}

func TestStatementExportCleanupRetainsUnattemptedShards(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	store := setupCustomerExportServiceTest(t)
	job, _, err := model.CreateCustomerExportJob(42, 42, model.CustomerExportJobTypeStatementDetails, model.CustomerExportFilters{})
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("statement-cleanup", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	artifact := &model.CustomerExportArtifact{StoreIdentity: store.ExportIdentity()}
	for i := range 12 {
		key := "test-part-" + strconv.Itoa(i)
		artifact.Files = append(artifact.Files, model.CustomerExportArtifactFile{ObjectKey: key})
		store.uploaded[key] = []byte("test output")
	}
	require.NoError(t, model.StageCustomerExportArtifact(context.Background(), job.JobID, artifact))
	require.NoError(t, model.FinishCustomerExportJob(job.JobID, claimed.Executor, model.CustomerExportJobStatusFailed, "generation_failed", "", nil))
	customerExportCleanupPass(context.Background())
	var remaining model.CustomerExportJob
	require.NoError(t, db.Where("job_id = ?", job.JobID).First(&remaining).Error)
	require.Len(t, remaining.DecodeArtifact().Files, 2)
	assert.Len(t, store.uploaded, 2)
	customerExportCleanupPass(context.Background())
	require.NoError(t, db.Where("job_id = ?", job.JobID).First(&remaining).Error)
	assert.Nil(t, remaining.DecodeArtifact())
	assert.Empty(t, store.uploaded)
}
