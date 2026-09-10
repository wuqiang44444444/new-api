package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	azurebatch "github.com/QuantumNous/new-api/relay/channel/azurebatch"
)

func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// RetrieveBatchFile loads one owned file row for metadata reads.
func RetrieveBatchFile(fileId string, userId int, appID int) (*model.BatchFile, error) {
	return model.GetBatchFileOwned(fileId, userId, appID)
}

// CancelBatchJob forwards a cancel request to the frozen channel and keeps
// tracking the job: cancellation is accepted upstream, but results still in
// flight remain chargeable and are settled from real facts.
func CancelBatchJob(jobId string, userId int, appID int) (*model.BatchJob, error) {
	job, err := model.GetBatchJobOwned(jobId, userId, appID)
	if err != nil {
		return nil, err
	}
	switch job.PublicStatus {
	case "completed", "failed", "expired", "cancelled":
		return job, nil
	case "cancelling":
		return job, nil
	}
	if job.UpstreamBatchId == "" {
		return nil, &dto.BatchValidateError{Message: "the batch job has not been accepted yet"}
	}
	client, err := batchClientForJob(job)
	if err != nil {
		return nil, err
	}
	ctx, cancel := contextWithTimeout()
	defer cancel()
	if _, err := client.CancelBatch(ctx, job.UpstreamBatchId); err != nil {
		if errors.Is(err, azurebatch.ErrUpstreamRejected) {
			// Azure may already have finished the job; keep polling instead
			// of failing the cancel call.
			common.SysLog("batch cancel was rejected upstream; continuing to track the job")
			return job, nil
		}
		return nil, fmt.Errorf("batch cancel could not be submitted")
	}
	if err := model.MarkBatchJobCancelRequested(job.Id); err != nil {
		return nil, err
	}
	return model.GetBatchJobOwned(job.Id, userId, appID)
}
