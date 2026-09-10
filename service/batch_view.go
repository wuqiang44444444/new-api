package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// batchJobObject projects the durable job row into the north-facing batch
// object. Internal channel, deployment, connection and money figures never
// enter the projection; request counters reflect the last trusted upstream
// observation, and file ids reference platform files, not upstream files.
func batchJobObject(job *model.BatchJob) *dto.BatchJobObject {
	object := &dto.BatchJobObject{
		Id:               job.Id,
		Object:           "batch",
		Endpoint:         job.Endpoint,
		Model:            job.PublicModel,
		Status:           job.PublicStatus,
		CompletionWindow: job.CompletionWindow,
		CreatedAt:        job.CreatedAt,
		ExpiresAt:        job.ExpiresAt,
		CancellingAt:     job.CancelRequestedAt,
		CancelledAt:      job.CancelledAt,
		FailedAt:         job.FailedAt,
		CompletedAt:      job.CompletedAt,
		InputFileId:      job.InputFileId,
	}
	if job.Metadata != "" {
		metadata := map[string]string{}
		if err := common.UnmarshalJsonStr(job.Metadata, &metadata); err == nil && len(metadata) > 0 {
			object.Metadata = metadata
		}
	}
	if job.UpstreamBatchId != "" || job.CountTotal > 0 {
		object.RequestCounts = &dto.BatchJobCounts{
			Total:     job.CountTotal,
			Completed: job.CountCompleted,
			Failed:    job.CountFailed,
			Expired:   job.CountExpired,
			Errored:   job.CountErrored,
			Cancelled: job.CountCancelled,
		}
	}
	if job.UsageTotal > 0 || job.UsageInput > 0 || job.UsageOutput > 0 {
		object.Usage = &dto.BatchJobUsage{
			InputTokens:  job.UsageInput,
			InputCached:  job.UsageCached,
			OutputTokens: job.UsageOutput,
			TotalTokens:  job.UsageTotal,
		}
	}
	// Result and error downloads publish only persisted platform files. A
	// cancelled or failed job with results still in collection reports its
	// terminal status without file ids until the objects exist here.
	if job.DeliveryState == model.BatchDeliveryReady {
		object.OutputFileId = job.OutputFileId
		object.ErrorFileId = job.ErrorFileId
	}
	if job.SanitizeError != "" {
		object.Errors = &dto.BatchErrors{Object: "list", Data: []dto.BatchErrorRef{{Code: "job_error", Message: job.SanitizeError}}}
	}
	return object
}

// BatchJobObjectView is the exported projection helper for controllers.
func BatchJobObjectView(job *model.BatchJob) *dto.BatchJobObject { return batchJobObject(job) }

func batchFileObject(file *model.BatchFile) *dto.BatchFileObject {
	return &dto.BatchFileObject{
		Id:        file.Id,
		Object:    "file",
		Bytes:     file.SizeBytes,
		CreatedAt: file.CreatedAt,
		Filename:  file.FileName,
		Purpose:   file.Purpose,
		Status:    "processed",
	}
}

// BatchFileObjectView is the exported projection helper for controllers.
func BatchFileObjectView(file *model.BatchFile) *dto.BatchFileObject { return batchFileObject(file) }
