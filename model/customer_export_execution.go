package model

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
)

var ErrCustomerExportCancelled = errors.New("customer export cancelled")

// CheckCustomerExportExecution fences both the worker and its current authority.
// Expired leases cannot be resurrected, even before recovery has run.
func CheckCustomerExportExecution(ctx context.Context, jobID, executor string, now int64) error {
	var job CustomerExportJob
	if err := DB.WithContext(ctx).Where("job_id = ?", jobID).First(&job).Error; err != nil {
		return err
	}
	if job.Executor != executor || job.LeaseUntil <= now ||
		(job.Status != CustomerExportJobStatusRunning && job.Status != CustomerExportJobStatusCancelWait) {
		return ErrCustomerExportStateConflict
	}
	if job.CancelRequested {
		return ErrCustomerExportCancelled
	}
	if err := AuthorizeCustomerExport(ctx, job.UserId, job.TargetUserId); err != nil {
		return err
	}
	if job.LeaseUntil >= now+119 {
		return nil
	}
	result := DB.WithContext(ctx).Model(&CustomerExportJob{}).
		Where("job_id = ? AND executor = ? AND status = ? AND lease_until > ? AND cancel_requested = ?", jobID, executor, CustomerExportJobStatusRunning, now, false).
		Updates(map[string]any{"lease_until": now + 120, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCustomerExportStateConflict
	}
	return nil
}

// StageCustomerExportArtifact records every intended object before sending a
// PUT. Failed, uncertain or interrupted uploads remain eligible for cleanup.
func StageCustomerExportArtifact(ctx context.Context, jobID string, artifact *CustomerExportArtifact) error {
	raw, err := common.Marshal(artifact)
	if err != nil {
		return err
	}
	result := DB.WithContext(ctx).Model(&CustomerExportJob{}).
		Where("job_id = ? AND status = ? AND cancel_requested = ? AND lease_until > ?", jobID, CustomerExportJobStatusRunning, false, common.GetTimestamp()).
		Updates(map[string]any{"artifact": string(raw), "updated_at": common.GetTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCustomerExportStateConflict
	}
	return nil
}
