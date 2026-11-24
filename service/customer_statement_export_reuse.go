package service

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"golang.org/x/sync/errgroup"
)

func findAvailableAzureStatementExport(ctx context.Context, store *azureBlobArtifactStore, actorID, targetID int, filters model.CustomerExportFilters, reuse model.CustomerExportReuse) (*model.CustomerExportJob, error) {
	legacy := filters
	legacy.FieldVersion = dailyBillingExportFieldVersion
	smallParts := legacy
	smallParts.StatementPartLimits = nil
	jobs, err := model.CompletedStatementExportCandidates(ctx, actorID, targetID, []model.CustomerExportFilters{filters, legacy, smallParts}, reuse)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		available, err := offerAzureStatementExport(ctx, store, job)
		if err != nil || available {
			return job, err
		}
	}
	return nil, nil
}

func offerAzureStatementExport(ctx context.Context, store *azureBlobArtifactStore, job *model.CustomerExportJob) (bool, error) {
	available, err := azureStatementExportAvailable(ctx, store, job)
	if err != nil || !available {
		return false, err
	}
	if err := model.AuthorizeCustomerExportJob(ctx, job); err != nil {
		return false, err
	}
	job.ConfirmExisting = true
	return true, nil
}

func azureStatementExportAvailable(ctx context.Context, store *azureBlobArtifactStore, job *model.CustomerExportJob) (bool, error) {
	now := common.GetTimestamp()
	artifact := job.DecodeArtifact()
	if job.JobType != model.CustomerExportJobTypeStatementDetails || job.Status != model.CustomerExportJobStatusSucceeded ||
		job.ExpiresAt <= now || artifact == nil || artifact.ExpiresAt <= now ||
		artifact.StoreIdentity != store.ExportIdentity() || len(artifact.Files) == 0 {
		return false, nil
	}
	checks, checkCtx := errgroup.WithContext(ctx)
	checks.SetLimit(4)
	var missing atomic.Bool
	for _, file := range artifact.Files {
		if checkCtx.Err() != nil {
			break
		}
		checks.Go(func() error {
			exists, err := store.ExportObjectExists(checkCtx, file.ObjectKey)
			if err == nil && !exists {
				missing.Store(true)
			}
			return err
		})
	}
	if err := checks.Wait(); err != nil {
		if missing.Load() {
			return false, nil
		}
		return false, errors.Join(ErrCustomerExportStorageUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return false, errors.Join(ErrCustomerExportStorageUnavailable, err)
	}
	return !missing.Load() && artifact.ExpiresAt > common.GetTimestamp() && job.ExpiresAt > common.GetTimestamp(), nil
}

// CheckExistingAzureStatementExport rechecks the offer at the user's download click.
func CheckExistingAzureStatementExport(ctx context.Context, job *model.CustomerExportJob) (bool, error) {
	store, err := currentExportObjectStore()
	azure, ok := store.(*azureBlobArtifactStore)
	if err != nil || !ok {
		return false, err
	}
	return azureStatementExportAvailable(ctx, azure, job)
}
