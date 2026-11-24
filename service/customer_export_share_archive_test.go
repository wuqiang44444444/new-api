package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strconv"
	"sync"
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

func TestCustomerExportShareArchivePublishesAndReusesExistingCSV(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	const sourceID = "cex_share_source"
	csv := []byte("quota\n10\n")
	objects := map[string][]byte{}
	artifact := model.CustomerExportArtifact{LineCount: 2, ExpiresAt: common.GetTimestamp() + 300}
	for _, name := range []string{"details.csv", "details.part2.csv"} {
		key := CustomerExportObjectNamespace + "/" + sourceID + "/" + name
		objects["/archives/"+key] = csv
		artifact.SizeBytes += int64(len(csv))
		artifact.Files = append(artifact.Files, model.CustomerExportArtifactFile{
			FileName: name, ObjectKey: key, SizeBytes: int64(len(csv)), Sha256: fmt.Sprintf("%x", sha256.Sum256(csv)),
		})
	}
	var mu sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			archiveJobID := path.Base(path.Dir(r.URL.Path))
			job, err := model.GetCustomerExportJob(archiveJobID)
			if assert.NoError(t, err) {
				assert.Equal(t, model.CustomerExportJobStatusRunning, job.Status)
				assert.Equal(t, &model.CustomerExportArchiveProgress{PackagedFiles: 2, TotalFiles: 2, Uploading: true}, job.DecodeProgress().Archive)
			}
			content, err := io.ReadAll(r.Body)
			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			objects[r.URL.Path] = content
			w.Header().Set("ETag", `"fixture"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet, http.MethodHead:
			content, ok := objects[r.URL.Path]
			if !ok {
				w.Header().Set("x-ms-error-code", "BlobNotFound")
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			if r.Method == http.MethodGet {
				_, _ = w.Write(content)
			}
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	config := system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendAzureBlob, Endpoint: server.URL, Bucket: "archives", AccountName: "fixture",
	}
	store, err := NewAzureBlobArtifactStore(config, "dGVzdC1rZXk=")
	require.NoError(t, err)
	azure := store.(*azureBlobArtifactStore)
	azure.client, err = azblob.NewClientWithSharedKeyCredential(config.Endpoint, azure.credential,
		&azblob.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: server.Client()}})
	require.NoError(t, err)
	customerExportStoreOverride = azure

	artifact.StoreIdentity = azure.ExportIdentity()
	manifest, err := common.Marshal(artifact)
	require.NoError(t, err)
	version := model.BillingStatementVersion{
		UserId: 42, DraftPublicId: "shared-confirmed", Status: model.BillingStatementVersionConfirmed,
		PeriodStart: 1785513600, PeriodEndExclusive: 1788192000,
	}
	require.NoError(t, db.Create(&version).Error)
	frozen, err := common.Marshal(model.CustomerExportFilters{
		StartTimestamp: version.PeriodStart, EndTimestamp: version.PeriodEndExclusive, Timezone: "Asia/Shanghai",
		StatementDraftId: version.DraftPublicId,
	})
	require.NoError(t, err)
	source := &model.CustomerExportJob{JobID: sourceID, UserId: 42, TargetUserId: 42,
		JobType: model.CustomerExportJobTypeStatementDetails, Status: model.CustomerExportJobStatusSucceeded,
		ExpiresAt: artifact.ExpiresAt, Artifact: string(manifest), Filters: string(frozen)}
	require.NoError(t, db.Create(source).Error)

	job, err := SubmitCustomerExportShareArchive(t.Context(), source)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusQueued, job.Status)
	claimed, err := model.ClaimNextQueuedCustomerExportJob("share-test", common.GetTimestamp()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, job.JobID, claimed.JobID)
	runCustomerExportJob(t.Context(), claimed, claimed.Executor, nil)

	finished, err := model.GetCustomerExportJob(job.JobID)
	require.NoError(t, err)
	require.Equal(t, model.CustomerExportJobStatusSucceeded, finished.Status)
	require.Greater(t, finished.ExpiresAt, source.ExpiresAt)
	link, expiresAt, err := PresignCustomerExportShareArchive(t.Context(), finished)
	require.NoError(t, err)
	parsed, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "r", parsed.Query().Get("sp"))
	assert.Equal(t, "https", parsed.Query().Get("spr"))
	assert.LessOrEqual(t, expiresAt, finished.ExpiresAt)
	assert.Greater(t, expiresAt, common.GetTimestamp()+int64((7*24*time.Hour-3*time.Minute)/time.Second))
	assert.NotEmpty(t, parsed.Query().Get("sig"))
	mu.Lock()
	data := objects[parsed.Path]
	mu.Unlock()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Len(t, reader.File, 2)
	for _, entry := range reader.File {
		payload, err := entry.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(payload)
		require.NoError(t, err)
		require.NoError(t, payload.Close())
		assert.Equal(t, csv, content)
	}

	again, err := SubmitCustomerExportShareArchive(t.Context(), source)
	require.NoError(t, err)
	assert.Equal(t, job.JobID, again.JobID)
	require.NoError(t, db.Model(&version).Update("status", model.BillingStatementVersionInvalid).Error)
	_, _, err = PresignCustomerExportShareArchive(t.Context(), finished)
	assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
}

func TestCustomerExportShareArchiveChecksFrozenVersionInHistoryAndLinks(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	setupCustomerExportServiceTest(t)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("role", common.RoleAdminUser).Error)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingExportTimezone())
	version := model.BillingStatementVersion{
		UserId: 42, DraftPublicId: "share-pending-version", Status: model.BillingStatementVersionPending,
		PeriodStart: start.Unix(), PeriodEndExclusive: start.AddDate(0, 1, 0).Unix(),
	}
	require.NoError(t, db.Create(&version).Error)
	filters := model.CustomerExportFilters{
		StartTimestamp: version.PeriodStart, EndTimestamp: version.PeriodEndExclusive,
		StatementDraftId: version.DraftPublicId,
	}
	sourceRaw, err := common.Marshal(filters)
	require.NoError(t, err)
	source := model.CustomerExportJob{JobID: "cex_share_draft_source", UserId: 42, TargetUserId: 42,
		JobType: model.CustomerExportJobTypeStatementDetails, Status: model.CustomerExportJobStatusSucceeded, Filters: string(sourceRaw)}
	require.NoError(t, db.Create(&source).Error)
	filters.StatementDraftId = ""
	filters.ArchiveSourceJobID = source.JobID
	for _, archivedVersion := range []*string{nil, &version.DraftPublicId} {
		filters.ArchiveDraftId = archivedVersion
		raw, err := common.Marshal(filters)
		require.NoError(t, err)
		jobID, err := model.GenerateCustomerExportJobID()
		require.NoError(t, err)
		job := model.CustomerExportJob{JobID: jobID, UserId: 42, TargetUserId: 42,
			JobType: model.CustomerExportJobTypeStatementArchive, Status: model.CustomerExportJobStatusSucceeded, Filters: string(raw)}
		require.NoError(t, db.Create(&job).Error)
		_, err = model.GetCustomerExportJobForOwner(job.JobID, 42)
		require.NoError(t, err)
	}
	history, total, err := model.ListCustomerExportJobs(t.Context(), 42, 1, 10)
	require.NoError(t, err)
	require.Len(t, history, 3)
	require.EqualValues(t, 3, total)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("role", common.RoleCommonUser).Error)
	for _, job := range history {
		_, err := model.GetCustomerExportJobForOwner(job.JobID, 42)
		assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	}
	jobs, total, err := model.ListCustomerExportJobs(t.Context(), 42, 1, 10)
	require.NoError(t, err)
	assert.Empty(t, jobs)
	assert.Zero(t, total)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, db.Model(&version).Update("status", model.BillingStatementVersionInvalid).Error)
	for _, job := range history {
		_, err := model.GetCustomerExportJobForOwner(job.JobID, 42)
		assert.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	}
}
