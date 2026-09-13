package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	azurebatch "github.com/QuantumNous/new-api/relay/channel/azurebatch"
	"github.com/shopspring/decimal"
	"io"
	"os"
)

// The batch progress handler owns the batch lifecycle. One SystemTask round
// polls every due job at least 60 seconds apart with backoff, collects result
// files for terminal jobs and settles from trusted line usage. Generic task
// timeout and refund scans must never touch batch jobs; progression and
// settlement happen only here.

const (
	batchPollInterval    = 60 * time.Second
	batchPollMaxFailures = 10
	batchPollLimit       = 10
)

type batchProgressHandler struct{}

func (batchProgressHandler) Type() string { return model.SystemTaskTypeBatchProgress }

func (batchProgressHandler) Enabled() bool { return true }

func (batchProgressHandler) Interval() time.Duration { return time.Minute }

func (batchProgressHandler) NewPayload() any { return struct{}{} }

func (h batchProgressHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	finishStatus := model.SystemTaskStatusSucceeded
	finishMessage := ""
	defer func() {
		if err := model.FinishSystemTask(task.TaskID, runnerID, finishStatus, nil, finishMessage); err != nil {
			common.SysError("batch scheduler completion failed")
		}
	}()
	due, err := model.GetDueBatchJobs(common.GetTimestamp(), batchPollLimit)
	if err != nil {
		finishStatus, finishMessage = model.SystemTaskStatusFailed, "batch jobs could not be loaded"
		common.SysError("batch progress load due jobs failed: " + err.Error())
		return
	}
	for _, job := range due {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := progressBatchJob(ctx, job); err != nil {
			common.SysError(fmt.Sprintf("batch job %s progress failed: %s", job.Id, err.Error()))
		}
	}
}

func init() {
	RegisterSystemTaskHandler(batchProgressHandler{})
}

func batchClientForJob(job *model.BatchJob) (*azurebatch.Client, error) {
	frozen, err := decodeBatchFrozenSnapshot(job.FrozenSnapshot)
	if err != nil {
		return nil, err
	}
	if frozen.AdapterVersion != azurebatch.AdapterVersion {
		return nil, errors.New("batch adapter version is unavailable")
	}
	connection := frozen.Connection
	if connection.BaseURL == "" || connection.Key == "" || connection.APIVersion == "" {
		return nil, errors.New("batch frozen connection is incomplete")
	}
	return azurebatch.NewClient(connection.BaseURL, connection.Key, connection.APIVersion), nil
}

// progressBatchJob runs one claimed observation cycle for a single job.
func progressBatchJob(ctx context.Context, job *model.BatchJob) error {
	now := common.GetTimestamp()
	nextPoll := now + int64(batchPollInterval.Seconds())*int64(1<<min(job.PollFailures, 5))
	claimed, err := model.ClaimBatchJobPoll(job.Id, job.PollAfter, job.PollFailures, job.PollVersion, nextPoll)
	if err != nil {
		return err
	}
	if !claimed {
		return nil // another worker owns this round
	}
	job.PollVersion++
	if job.DeliveryState == model.BatchDeliveryReady {
		return settlePersistedBatchJob(ctx, job)
	}
	client, err := batchClientForJob(job)
	if err != nil {
		return err
	}
	status, err := client.RetrieveBatch(ctx, job.UpstreamBatchId)
	if err != nil {
		if job.PollFailures+1 >= batchPollMaxFailures {
			common.SysError(fmt.Sprintf("batch job %s polling keeps failing; keeping last trusted state", job.Id))
		}
		// A single failed observation never fails the job: the claim already
		// scheduled the next attempt with an incremented failure count.
		return nil
	}
	return applyBatchObservation(ctx, job, status, job.PollVersion)
}

// applyBatchObservation persists one trusted observation, collects terminal
// results and settles.
func applyBatchObservation(ctx context.Context, job *model.BatchJob, status *azurebatch.BatchStatus, claimedVersion int64) error {
	if status == nil || status.Id != job.UpstreamBatchId {
		return errors.New("batch observation identity mismatch")
	}
	job.UpstreamStatus = status.Status
	job.PublicStatus = batchPublicStatusForUpstream(status.Status)
	job.CountTotal = status.CountTotal
	job.CountCompleted = status.CountCompleted
	job.CountFailed = status.CountFailed
	job.CountExpired = status.CountExpired
	job.CountErrored = status.CountErrored
	job.CountCancelled = status.CountCancelled
	job.ExpiresAt = status.ExpiresAt
	job.CancelledAt = status.CancelledAt
	job.FailedAt = status.FailedAt
	job.CompletedAt = status.CompletedAt
	if status.SanitizeError != "" {
		job.SanitizeError = status.SanitizeError
	}

	terminal := status.Terminal()
	if terminal && (status.OutputFileId != "" || status.ErrorFileId != "") {
		job.DeliveryState = model.BatchDeliveryProcessing
	}
	if err := model.SaveBatchJobObservation(job, claimedVersion); err != nil {
		return err
	}
	if !terminal {
		return nil
	}
	return collectAndSettleBatchJob(ctx, job, status)
}

// collectAndSettleBatchJob downloads persisted results, records line facts,
// computes the per-line settlement target and applies it atomically. Delivery
// and settlement recover independently; neither re-runs inference.
func collectAndSettleBatchJob(ctx context.Context, job *model.BatchJob, status *azurebatch.BatchStatus) error {
	frozen, err := decodeBatchFrozenSnapshot(job.FrozenSnapshot)
	if err != nil {
		return err
	}
	client, err := batchClientForJob(job)
	if err != nil {
		return err
	}

	var lineUsage []azurebatch.LineResult
	for _, result := range []struct{ upstream, purpose string }{
		{status.OutputFileId, "batch_output"}, {status.ErrorFileId, "batch_error"},
	} {
		if result.upstream == "" {
			continue
		}
		lines, err := collectBatchResult(ctx, job, client, result.upstream, result.purpose)
		if err != nil {
			return err
		}
		lineUsage = append(lineUsage, lines...)
	}
	if err := validateBatchResultCompleteness(job, status, frozen, lineUsage); err != nil {
		return err
	}
	facts := make([]model.BatchJobLine, 0, len(lineUsage))
	for _, line := range lineUsage {
		quota, finalQuota := 0, 0
		var clamp *common.QuotaClamp
		if line.Status == "completed" {
			var err error
			quota, clamp, err = computeBatchLineModelQuota(frozen, BatchLineUsage{CustomId: line.CustomId, InputTokens: line.InputTokens, OutputTokens: line.OutputTokens, CachedTokens: line.CachedTokens})
			if err != nil {
				return err
			}
			modelClamp := clamp
			finalQuota, clamp, err = computeBatchLineFinalQuota(frozen, BatchLineUsage{CustomId: line.CustomId, InputTokens: line.InputTokens, OutputTokens: line.OutputTokens, CachedTokens: line.CachedTokens})
			if err != nil {
				return err
			}
			if clamp == nil {
				clamp = modelClamp
			}
		}
		clampJSON := ""
		if clamp != nil {
			data, err := common.Marshal(clamp)
			if err != nil {
				return err
			}
			clampJSON = string(data)
		}
		facts = append(facts, model.BatchJobLine{JobId: job.Id, CustomId: line.CustomId, Status: line.Status,
			InputTokens: line.InputTokens, OutputTokens: line.OutputTokens, CachedTokens: line.CachedTokens,
			TotalTokens: line.TotalTokens, ModelQuota: quota, FinalQuota: finalQuota, ErrorCode: line.ErrorCode, QuotaClamp: clampJSON})
	}
	if err := model.CommitBatchResultLines(job, facts); err != nil {
		return err
	}
	return settlePersistedBatchJob(ctx, job)
}

// Complete line facts are sufficient for funding/log recovery, even when the
// provider or private result objects are no longer available.
func settlePersistedBatchJob(ctx context.Context, job *model.BatchJob) error {
	target, err := settleBatchJobTarget(job)
	if err != nil {
		return err
	}
	if err := applyBatchSettlement(ctx, job, target); err != nil {
		return err
	}
	return nil
}

// Trusted counts account for unexecuted requests without inventing result rows.
// Successful requests always require their own complete usage evidence.
func validateBatchResultCompleteness(job *model.BatchJob, status *azurebatch.BatchStatus, frozen *model.BatchFrozenSnapshot, lines []azurebatch.LineResult) error {
	// Azure's failed state denotes input validation failure, before inference.
	// Require its structured error evidence rather than inferring this from an
	// absent output file or a failed poll.
	if status.ValidationFailed && len(lines) == 0 {
		return nil
	}
	if !status.CountsPresent || status.CountTotal != job.LineCount {
		return errors.New("batch request counts are incomplete")
	}
	seen := make(map[string]bool, len(lines))
	var completed, failed int64
	for _, line := range lines {
		if _, ok := frozen.LineInputs[line.CustomId]; !ok || seen[line.CustomId] {
			return errors.New("batch result identity is unknown or duplicated")
		}
		seen[line.CustomId] = true
		if line.Status == "completed" {
			completed++
		} else {
			failed++
		}
	}
	if completed != status.CountCompleted || failed > status.CountTotal-completed {
		return errors.New("batch success evidence does not match request counts")
	}
	accounted := status.CountCompleted + status.CountFailed + status.CountExpired + status.CountCancelled
	if accounted > status.CountTotal || (accounted != status.CountTotal && status.Status != azurebatch.StatusCancelled && status.Status != azurebatch.StatusExpired) {
		return errors.New("batch final counters do not account for every request")
	}
	if status.Status == azurebatch.StatusCompleted && failed != status.CountFailed {
		return errors.New("batch failure evidence is incomplete")
	}
	return nil
}

// A persisted file is reused on retries. Local spooling bounds memory while
// retaining the complete original provider result for authenticated delivery.
func collectBatchResult(ctx context.Context, job *model.BatchJob, client *azurebatch.Client, upstreamID, purpose string) ([]azurebatch.LineResult, error) {
	spool, err := os.CreateTemp("", "batch-result-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(spool.Name())
	defer spool.Close()
	fileID := job.OutputFileId
	if purpose == "batch_error" {
		fileID = job.ErrorFileId
	}
	var row *model.BatchFile
	if fileID != "" {
		row, err = model.GetBatchFileOwned(fileID, job.UserId, job.AppID)
		if err == nil {
			_, err = GetBatchObject(ctx, row.ObjectKey, spool)
		}
	} else {
		var body io.ReadCloser
		body, err = client.OpenFile(ctx, upstreamID)
		if err == nil {
			_, err = io.Copy(spool, body)
			body.Close()
		}
	}
	if err != nil {
		return nil, errors.New("batch result download failed")
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	lines, err := azurebatch.ParseResultLines(spool)
	if err != nil {
		return nil, err
	}
	if row == nil {
		if _, err = spool.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		delivery, err := os.CreateTemp("", "batch-delivery-*.jsonl")
		if err != nil {
			return nil, err
		}
		defer os.Remove(delivery.Name())
		defer delivery.Close()
		if err := writeBatchPublicResult(delivery, spool, job.PublicModel); err != nil {
			return nil, err
		}
		if _, err := delivery.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		// A stale lease must never overwrite the object referenced by a newer
		// observation. Only the winning version's immutable object is attached.
		key := batchObjectKey(job.UserId, fmt.Sprintf("%s-%s-%d", job.Id, purpose, job.PollVersion))
		size, err := PutBatchObject(ctx, key, delivery)
		if err != nil {
			return nil, err
		}
		row, err = model.AttachBatchResultFile(job, purpose, key, size, int64(len(lines)))
		if err != nil {
			return nil, err
		}
	}
	if purpose == "batch_output" {
		job.OutputFileId = row.Id
		job.OutputObjectKey = row.ObjectKey
	} else {
		job.ErrorFileId = row.Id
		job.ErrorObjectKey = row.ObjectKey
	}
	return lines, nil
}

// settleBatchJobTarget sums recorded line quotas. Lines the upstream counted
// as failed/expired/cancelled without any usage contribute zero: they have
// explicit zero-fee evidence.
func settleBatchJobTarget(job *model.BatchJob) (int, error) {
	total, err := model.SumBatchJobQuota(job.Id)
	if err != nil {
		return 0, err
	}
	quota, clamp := common.QuotaFromDecimalChecked(decimal.NewFromInt(total))
	if clamp != nil {
		return 0, errors.New("batch total exceeds supported quota range; reconciliation required")
	}
	return quota, nil
}

// applyBatchSettlement moves the task funds from the frozen estimate to the
// trusted target through the shared atomic settlement primitive.
func applyBatchSettlement(ctx context.Context, job *model.BatchJob, target int) error {
	task, err := model.GetTaskById(job.TaskRowId)
	if err != nil {
		return err
	}
	applied, _, err := model.ApplyTaskBillingTarget(task, target)
	if err != nil {
		if errors.Is(err, model.ErrTaskBillingInsufficientFunding) {
			return model.MarkBatchJobSettleState(job, model.BatchSettleDebt)
		}
		_ = model.MarkBatchJobSettleState(job, model.BatchSettleFailed)
		return err
	}
	_ = applied

	other := taskBillingOther(task)
	other.SetPublic("task_id", task.TaskID)
	other.SetPublic("billing_mode", "azure_batch")
	other.SetPublic("task_billing_event", "create")
	other.SetPublic("batch_line_count", job.LineCount)
	lines, err := model.ListBatchJobLines(job.Id)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if line.QuotaClamp != "" {
			var clamp common.QuotaClamp
			if err := common.UnmarshalJsonStr(line.QuotaClamp, &clamp); err != nil {
				return err
			}
			attachQuotaSaturationToOther(other, &clamp)
			logger.LogWarn(ctx, "batch quota saturation task="+task.TaskID)
			break
		}
	}
	var input, output decimal.Decimal
	for _, line := range lines {
		input = input.Add(decimal.NewFromInt(line.InputTokens))
		output = output.Add(decimal.NewFromInt(line.OutputTokens))
	}
	promptTokens, inputClamp := common.QuotaFromDecimalChecked(input)
	completionTokens, outputClamp := common.QuotaFromDecimalChecked(output)
	for _, clamp := range []*common.QuotaClamp{inputClamp, outputClamp} {
		if clamp != nil {
			other.SetAdmin("token_saturation", clamp.AuditMap())
			logger.LogWarn(ctx, "batch log token saturation task="+task.TaskID)
		}
	}
	return model.CompleteBatchSettlement(job, task, target, other, promptTokens, completionTokens)
}

func decodeBatchFrozenSnapshot(raw string) (*model.BatchFrozenSnapshot, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("batch job has no frozen snapshot")
	}
	var frozen model.BatchFrozenSnapshot
	if err := common.UnmarshalJsonStr(raw, &frozen); err != nil {
		return nil, err
	}
	return &frozen, nil
}
