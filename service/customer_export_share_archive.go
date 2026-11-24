package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/shirou/gopsutil/disk"
)

const customerExportShareRetention = 7 * 24 * time.Hour
const customerExportShareFileName = "statement-details.zip"

func SubmitCustomerExportShareArchive(ctx context.Context, source *model.CustomerExportJob) (*model.CustomerExportJob, error) {
	if source.JobType != model.CustomerExportJobTypeStatementDetails ||
		source.Status != model.CustomerExportJobStatusSucceeded || source.ExpiresAt <= common.GetTimestamp() {
		return nil, ErrCustomerExportArchiveUnavailable
	}
	archive, err := PrepareCustomerExportArchive(source)
	if err != nil {
		return nil, err
	}
	azure, ok := archive.store.(*azureBlobArtifactStore)
	if !ok || !strings.HasPrefix(strings.ToLower(azure.config.Endpoint), "https://") {
		return nil, ErrCustomerExportStorageUnavailable
	}
	original, err := source.DecodeFilters()
	if err != nil {
		return nil, err
	}
	filters := model.CustomerExportFilters{
		ArchiveSourceJobID: source.JobID, FieldVersion: customerExportFormatVersion(model.CustomerExportJobTypeStatementArchive),
		ArchiveDraftId: &original.StatementDraftId,
		StartTimestamp: original.StartTimestamp, EndTimestamp: original.EndTimestamp, Timezone: original.Timezone, Language: original.Language,
	}
	if err := model.AuthorizeCustomerExportJob(ctx, source); err != nil {
		return nil, err
	}
	job, created, err := model.CreateCustomerExportJob(source.UserId, source.TargetUserId, model.CustomerExportJobTypeStatementArchive, filters,
		model.CustomerExportReuse{SourceVersion: source.JobID, StoreIdentity: archive.store.ExportIdentity()})
	if err == nil && created {
		if _, _, wakeErr := EnqueueSystemTask(SystemTaskTypeCustomerExport, nil); wakeErr != nil {
			logger.LogWarn(ctx, "customer export archive scheduler wake failed")
		}
	}
	return job, err
}

func executeCustomerExportShareArchive(ctx context.Context, job *model.CustomerExportJob, filters model.CustomerExportFilters, workDir string) (*model.CustomerExportArtifact, error) {
	ctx, cancel := context.WithTimeout(ctx, customerExportJobBudget)
	defer cancel()
	source, err := model.GetCustomerExportJobForOwner(filters.ArchiveSourceJobID, job.UserId)
	if err != nil {
		return nil, err
	}
	if source.JobType != model.CustomerExportJobTypeStatementDetails || source.TargetUserId != job.TargetUserId ||
		source.Status != model.CustomerExportJobStatusSucceeded || source.ExpiresAt <= common.GetTimestamp() {
		return nil, ErrCustomerExportArchiveUnavailable
	}
	archive, err := PrepareCustomerExportArchive(source)
	if err != nil {
		return nil, err
	}
	azure, ok := archive.store.(*azureBlobArtifactStore)
	if !ok || !strings.HasPrefix(strings.ToLower(azure.config.Endpoint), "https://") {
		return nil, ErrCustomerExportStorageUnavailable
	}
	sourceArtifact := source.DecodeArtifact()
	space, err := disk.UsageWithContext(ctx, os.TempDir())
	if err != nil || sourceArtifact.SizeBytes <= 0 || space.Free <= 256<<20 || uint64(sourceArtifact.SizeBytes) > space.Free-(256<<20) {
		return nil, errCustomerExportBudgetExceeded
	}
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(workDir, "statement-*.zip")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	digest := sha256.New()
	select {
	case customerExportArchiveSlot <- struct{}{}:
		defer func() { <-customerExportArchiveSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	progress := model.CustomerExportArchiveProgress{TotalFiles: len(archive.files)}
	var lastProgressWrite time.Time
	publishProgress := func() {
		lastProgressWrite = time.Now()
		progressCtx, cancel := context.WithTimeout(ctx, customerExportBatchTimeout)
		defer cancel()
		if err := model.UpdateCustomerExportProgress(progressCtx, job.JobID, job.Executor, model.CustomerExportProgress{Archive: &progress}); err != nil && ctx.Err() == nil {
			logger.LogWarn(ctx, "customer export archive progress write failed")
		}
	}
	publishProgress()
	if err := archive.writeZIP(ctx, io.MultiWriter(file, digest), func(packagedFiles int) {
		progress.PackagedFiles = packagedFiles
		if packagedFiles == 1 || time.Since(lastProgressWrite) >= customerExportProgressInterval {
			publishProgress()
		}
	}); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	key := CustomerExportObjectNamespace + "/" + job.JobID + "/" + customerExportShareFileName
	artifact := &model.CustomerExportArtifact{
		SourceVersion: source.JobID, StoreIdentity: archive.store.ExportIdentity(),
		LineCount: sourceArtifact.LineCount, SizeBytes: info.Size(), MissingDates: sourceArtifact.MissingDates,
		Files: []model.CustomerExportArtifactFile{{
			ObjectKey: key, FileName: customerExportShareFileName, SizeBytes: info.Size(),
			Sha256: fmt.Sprintf("%x", digest.Sum(nil)), LineCount: sourceArtifact.LineCount,
		}},
	}
	if err := model.StageCustomerExportArtifact(ctx, job.JobID, artifact); err != nil {
		return nil, err
	}
	progress.Uploading = true
	publishProgress()
	if err := azure.putObjectFile(ctx, key, "application/zip", file.Name(), info.Size(), "customer_export_archive"); err != nil {
		return nil, ErrCustomerExportStorageUnavailable
	}
	now := time.Now()
	artifact.GeneratedAt = now.Unix()
	artifact.ExpiresAt = now.Add(customerExportShareRetention).Unix()
	return artifact, nil
}

// PresignCustomerExportShareArchive signs exactly the retained private ZIP.
func PresignCustomerExportShareArchive(ctx context.Context, job *model.CustomerExportJob) (string, int64, error) {
	if job.JobType != model.CustomerExportJobTypeStatementArchive || job.Status != model.CustomerExportJobStatusSucceeded {
		return "", 0, ErrCustomerExportArchiveUnavailable
	}
	artifact := job.DecodeArtifact()
	if artifact == nil || len(artifact.Files) != 1 {
		return "", 0, ErrCustomerExportArchiveUnavailable
	}
	store, err := currentExportObjectStore()
	azure, ok := store.(*azureBlobArtifactStore)
	if err != nil || !ok || artifact.StoreIdentity != azure.ExportIdentity() ||
		!strings.HasPrefix(strings.ToLower(azure.config.Endpoint), "https://") {
		return "", 0, ErrCustomerExportStorageUnavailable
	}
	key := CustomerExportObjectNamespace + "/" + job.JobID + "/" + customerExportShareFileName
	if artifact.Files[0].ObjectKey != key {
		return "", 0, ErrCustomerExportArchiveUnavailable
	}
	now := time.Now().UTC()
	start := now.Add(-azureSASClockSkewBackoff)
	expires := time.Unix(min(job.ExpiresAt, artifact.ExpiresAt, start.Add(customerExportShareRetention).Unix()), 0)
	if !expires.After(now) {
		return "", 0, ErrCustomerExportArchiveUnavailable
	}
	exists, err := azure.ExportObjectExists(ctx, key)
	if err != nil || !exists {
		return "", 0, ErrCustomerExportArchiveUnavailable
	}
	if err := model.AuthorizeCustomerExportJob(ctx, job); err != nil {
		return "", 0, err
	}
	query, err := (sas.BlobSignatureValues{
		Protocol: sas.ProtocolHTTPS, StartTime: start, ExpiryTime: expires,
		Permissions:   (&sas.BlobPermissions{Read: true}).String(),
		ContainerName: azure.container, BlobName: azure.prefixedKey(key),
		ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": customerExportShareFileName}),
	}).SignWithSharedKey(azure.credential)
	if err != nil {
		return "", 0, ErrCustomerExportStorageUnavailable
	}
	return azure.blobClient(key).URL() + "?" + query.Encode(), expires.Unix(), nil
}
