package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
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

func TestCustomerStatementDayRecoversInterruptedPartAndHonorsDiskBudget(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	store := setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, billingExportTimezone()).Unix()
	requestIDs := []string{"first", "second", "third", "fourth"}
	for i, id := range requestIDs {
		require.NoError(t, db.Create(&model.Log{
			UserId: 42, CreatedAt: start + int64(i+1), Type: model.LogTypeConsume, Quota: i + 1, RequestId: id,
		}).Error)
	}
	publishBillingExportTestDay(t, start)
	request := CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 86400,
	}
	job, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	filters, err := job.DecodeFilters()
	require.NoError(t, err)
	bound, _, err := model.ResolveBillingReport(t.Context(), start, start+86400-1, filters.BillingReportVersion)
	require.NoError(t, err)
	claim := func(executor string) *model.CustomerExportJob {
		t.Helper()
		claimed, err := model.ClaimNextQueuedCustomerExportJob(executor, common.GetTimestamp()+120, 1800)
		require.NoError(t, err)
		require.NotNil(t, claimed)
		return claimed
	}
	policy := customerStatementExportPolicy{beforeBatch: func(context.Context) error { return nil }}
	claimed := claim("large-part-interrupted")
	ctx, cancel := context.WithCancel(bound)
	defer cancel()
	customerExportStoreOverride = &interruptedStatementExportStore{store, cancel}
	_, err = executeCustomerStatementExport(ctx, claimed, filters, t.TempDir(), policy)
	require.ErrorIs(t, err, context.Canceled)
	interrupted, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	pending := interrupted.DecodeArtifact()
	require.NotNil(t, pending)
	require.True(t, pending.Checkpoint.PendingFile)
	require.Len(t, pending.Files, 1)
	require.EqualValues(t, 4, pending.Files[0].LineCount)
	require.Zero(t, pending.Checkpoint.Cursor)
	customerExportStoreOverride = store
	require.NoError(t, db.Model(job).Update("lease_until", common.GetTimestamp()-1).Error)
	recovered, err := model.RecoverInterruptedCustomerExportJobs(common.GetTimestamp())
	require.NoError(t, err)
	require.EqualValues(t, 1, recovered)
	claimed = claim("large-part-recovered")
	// A different worker cannot even fit the old pending file on disk.
	budget := pending.Files[0].SizeBytes - 1
	policy.diskBudget = func(_ context.Context, path string) (int64, error) {
		_, err := os.Stat(path)
		return budget, err
	}
	artifact, err := executeCustomerStatementExport(bound, claimed, filters, t.TempDir(), policy)
	require.NoError(t, err)
	require.Len(t, artifact.Files, 2)
	assert.Equal(t, []string{pending.Files[0].ObjectKey}, artifact.ObsoleteKeys)
	assert.EqualValues(t, 4, artifact.LineCount)
	assert.EqualValues(t, 10, artifact.StatementTotals.Net)
	var actual []string
	for _, file := range artifact.Files {
		assert.LessOrEqual(t, file.SizeBytes, budget)
		assert.NotEqual(t, pending.Files[0].ObjectKey, file.ObjectKey)
		rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(store.uploaded[file.ObjectKey], []byte(customerExportCsvBOM)))).ReadAll()
		require.NoError(t, err)
		for _, row := range rows[1:] {
			actual = append(actual, reconciliationTestCells(t, customerStatementDetailColumns, row)["request_id"])
		}
	}
	assert.Equal(t, requestIDs, actual)
	require.NoError(t, model.FinishCustomerExportJob(job.JobID, claimed.Executor, model.CustomerExportJobStatusSucceeded, "", "", artifact))
	require.NoError(t, db.Model(job).Update("expires_at", common.GetTimestamp()-1).Error)
	customerExportCleanupPass(t.Context())
	assert.Contains(t, store.deleted, pending.Files[0].ObjectKey)

	// A fresh job with the same disk headroom must seal before exceeding it.
	request.ForceNew = true
	job, err = SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	claimed = claim("disk-limited-part")
	require.Equal(t, job.JobID, claimed.JobID)
	limited, err := executeCustomerStatementExport(bound, claimed, filters, t.TempDir(), policy)
	require.NoError(t, err)
	require.Len(t, limited.Files, 2)
	assert.EqualValues(t, 4, limited.LineCount)
	for _, file := range limited.Files {
		assert.LessOrEqual(t, file.SizeBytes, budget)
	}
}

func TestCustomerStatementDayCountsWrittenRowsWhenFiltering(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, billingExportTimezone()).Unix()
	for i, name := range []string{"selected", "other", "other", "selected"} {
		require.NoError(t, db.Create(&model.Log{
			UserId: 42, CreatedAt: start + int64(i+1), Type: model.LogTypeConsume, Quota: 1, ModelName: name,
		}).Error)
	}
	publishBillingExportTestDay(t, start)
	job, err := SubmitCustomerExportJob(42, 42, CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 86400, ModelName: "selected",
	})
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("filtered-daily-part", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	runCustomerExportJob(t.Context(), claimed, claimed.Executor, &customerExportPressureTracker{
		statementPolicy: customerStatementExportPolicy{partRows: 2, beforeBatch: func(context.Context) error { return nil }},
	})
	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	artifact := finished.DecodeArtifact()
	require.NotNil(t, artifact)
	require.Len(t, artifact.Files, 1)
	assert.EqualValues(t, 2, artifact.Files[0].LineCount)
}

func TestCustomerStatementDayOffersOnlyAvailableAzureFilesAndCanForceNew(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))
	config := system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendAzureBlob, Endpoint: "https://fixture.invalid", Bucket: "archives", AccountName: "fixture",
	}
	fileStore, err := NewAzureBlobArtifactStore(config, "Zml4dHVyZQ==")
	require.NoError(t, err)
	azure := fileStore.(*azureBlobArtifactStore)
	var secondMissing atomic.Bool
	var checked atomic.Int64
	azure.client, err = azblob.NewClientWithSharedKeyCredential(config.Endpoint, azure.credential, &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: statementExportUploadTransport(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, http.MethodHead, req.Method)
			checked.Add(1)
			response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: req}
			if secondMissing.Load() && strings.HasSuffix(req.URL.Path, ".part2.csv") {
				response.StatusCode = http.StatusNotFound
				response.Header.Set("x-ms-error-code", "BlobNotFound")
			}
			return response, nil
		})},
	})
	require.NoError(t, err)
	customerExportStoreOverride = azure
	start := time.Date(2026, 8, 2, 0, 0, 0, 0, billingExportTimezone()).Unix()
	require.NoError(t, db.Create(&model.Log{
		UserId: 42, CreatedAt: start + 1, Type: model.LogTypeConsume, Quota: 50, RequestId: "daily",
	}).Error)
	publishBillingExportTestDay(t, start)
	request := CustomerExportRequest{
		JobType: model.CustomerExportJobTypeStatementDetails, StartTimestamp: start, EndTimestamp: start + 86400,
	}
	old, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	filters, err := old.DecodeFilters()
	require.NoError(t, err)
	filters.FieldVersion = dailyBillingExportFieldVersion
	filters.StatementPartLimits = nil
	raw, err := common.Marshal(filters)
	require.NoError(t, err)
	require.NoError(t, db.Model(old).Update("filters", string(raw)).Error)
	source, err := model.CustomerStatementExportSourceVersion(t.Context(), 42, request.JobType, filters)
	require.NoError(t, err)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("azure-confirmation", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	artifact := &model.CustomerExportArtifact{
		SourceVersion: source, StoreIdentity: azure.ExportIdentity(), ExpiresAt: common.GetTimestamp() + 300,
		Files: []model.CustomerExportArtifactFile{
			{ObjectKey: "exports/jobs/" + old.JobID + "/details.csv"},
			{ObjectKey: "exports/jobs/" + old.JobID + "/details.part2.csv"},
		},
	}
	require.NoError(t, model.FinishCustomerExportJob(old.JobID, claimed.Executor, model.CustomerExportJobStatusSucceeded, "", "", artifact))
	request.ConfirmExisting = true
	offer, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	assert.Equal(t, old.JobID, offer.JobID)
	assert.True(t, offer.ToView().ConfirmExisting)
	assert.EqualValues(t, 2, checked.Load())

	request.ConfirmExisting, request.ForceNew = false, true
	fresh, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	assert.NotEqual(t, old.JobID, fresh.JobID)
	assert.False(t, fresh.ToView().ConfirmExisting)
	assert.EqualValues(t, 2, checked.Load())
	_, err = model.CancelCustomerExportJob(fresh.JobID, 42)
	require.NoError(t, err)

	secondMissing.Store(true)
	available, err := CheckExistingAzureStatementExport(t.Context(), offer)
	require.NoError(t, err)
	assert.False(t, available)
	request.ConfirmExisting, request.ForceNew = true, false
	replacement, err := SubmitCustomerExportJob(42, 42, request)
	require.NoError(t, err)
	assert.NotEqual(t, old.JobID, replacement.JobID)
	assert.False(t, replacement.ToView().ConfirmExisting)
}
