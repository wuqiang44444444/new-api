package model

import (
	"errors"
	"math"
	"reflect"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func lockBatchPollTx(tx *gorm.DB, job *BatchJob) (*BatchJob, error) {
	var current BatchJob
	if err := lockForUpdate(tx).First(&current, "id = ?", job.Id).Error; err != nil {
		return nil, err
	}
	if current.PollVersion != job.PollVersion {
		return nil, errors.New("batch poll claim expired")
	}
	return &current, nil
}

// AttachBatchResultFile commits the file reference under the current claim.
// A retry reuses the same reference instead of creating another customer file.
func AttachBatchResultFile(job *BatchJob, purpose, key string, size, count int64) (*BatchFile, error) {
	var file BatchFile
	err := DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockBatchPollTx(tx, job)
		if err != nil {
			return err
		}
		idColumn, keyColumn, id := "output_file_id", "output_object_key", current.OutputFileId
		if purpose == "batch_error" {
			idColumn, keyColumn, id = "error_file_id", "error_object_key", current.ErrorFileId
		}
		if id != "" {
			return tx.First(&file, "id = ?", id).Error
		}
		id, err = newBatchFileId()
		if err != nil {
			return err
		}
		file = BatchFile{Id: id, UserId: job.UserId, AppID: job.AppID, TokenId: job.TokenId,
			FileName: job.Id + "_" + purpose + ".jsonl", Purpose: purpose, SizeBytes: size, LineCount: count,
			ModelName: job.PublicModel, ObjectKey: key, UploadSource: "batch_result", CreatedAt: common.GetTimestamp()}
		if err := tx.Create(&file).Error; err != nil {
			return err
		}
		return tx.Model(current).Updates(map[string]any{idColumn: id, keyColumn: key}).Error
	})
	return &file, err
}

// CommitBatchResultLines commits complete immutable evidence in bounded batches.
// A conflicting retry cannot silently replace or ignore a billed line.
func CommitBatchResultLines(job *BatchJob, lines []BatchJobLine) error {
	var usage [4]int64
	for _, line := range lines {
		for i, value := range []int64{line.InputTokens, line.CachedTokens, line.OutputTokens, line.TotalTokens} {
			if value < 0 || usage[i] > math.MaxInt64-value {
				return errors.New("batch aggregate usage exceeds supported range")
			}
			usage[i] += value
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockBatchPollTx(tx, job)
		if err != nil {
			return err
		}
		var existing []BatchJobLine
		if err := tx.Where("job_id = ?", job.Id).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if len(existing) != len(lines) {
				return errors.New("batch result evidence changed")
			}
			byID := make(map[string]BatchJobLine, len(existing))
			for _, row := range existing {
				row.Id = 0
				row.CreatedAt = 0
				byID[row.CustomId] = row
			}
			for _, row := range lines {
				if !reflect.DeepEqual(byID[row.CustomId], row) {
					return errors.New("batch result evidence conflicts")
				}
			}
		} else if len(lines) > 0 {
			if err := tx.CreateInBatches(lines, 50).Error; err != nil {
				return err
			}
		}
		publicStatus := current.PublicStatus
		if current.UpstreamStatus == "completed" {
			publicStatus = "completed"
		}
		return tx.Model(current).Updates(map[string]any{"delivery_state": BatchDeliveryReady, "public_status": publicStatus,
			"usage_input": usage[0], "usage_cached": usage[1], "usage_output": usage[2], "usage_total": usage[3]}).Error
	})
}

func SumBatchJobQuota(jobID string) (int64, error) {
	var sum int64
	err := DB.Model(&BatchJobLine{}).Select("COALESCE(SUM(final_quota), 0)").Where("job_id = ?", jobID).Scan(&sum).Error
	return sum, err
}

// The task's settled balance is durable before log projection. The job stays
// pending until its deterministic log is present, so a crash can retry safely.
func CompleteBatchSettlement(job *BatchJob, task *Task, target int, other *LogOther, promptTokens, completionTokens int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		current, err := lockBatchPollTx(tx, job)
		if err != nil {
			return err
		}
		if current.SettleState == BatchSettleSettled {
			return nil
		}
		if err := tx.Model(&User{}).Where("id = ?", task.UserId).Updates(map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", target),
			"request_count": gorm.Expr("request_count + ?", current.LineCount),
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Channel{}).Where("id = ?", task.ChannelId).
			Update("used_quota", gorm.Expr("used_quota + ?", target)).Error; err != nil {
			return err
		}
		logs := LOG_DB
		if LOG_DB == DB {
			logs = tx
		}
		var count int64
		if err := logs.Model(&Log{}).Where("request_id = ? AND type = ?", job.Id, LogTypeConsume).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			row := Log{UserId: task.UserId, CreatedAt: common.GetTimestamp(), Type: LogTypeConsume,
				PromptTokens: promptTokens, CompletionTokens: completionTokens,
				ChannelId: task.ChannelId, ModelName: task.Properties.OriginModelName, Quota: target,
				TokenId: task.PrivateData.TokenId, Group: task.Group, RequestId: job.Id, Other: other.JSONString()}
			if err := logs.Create(&row).Error; err != nil {
				return err
			}
		}
		if err := recordBatchQuotaDataTx(tx, current, task, target, promptTokens, completionTokens); err != nil {
			return err
		}
		// Commit the terminal Task projection with the completed job. Otherwise a
		// crash after completing the job removes it from polling before the Task
		// can reach its terminal state.
		status := TaskStatusFailure
		if current.PublicStatus == "completed" {
			status = TaskStatusSuccess
		} else if current.PublicStatus == "cancelled" {
			status = TaskStatusCancelled
		}
		if err := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status": status, "progress": "100%", "finish_time": common.GetTimestamp(),
			"fail_reason": current.SanitizeError,
		}).Error; err != nil {
			return err
		}
		return tx.Model(current).Updates(map[string]any{"settle_state": BatchSettleSettled, "target_quota": target}).Error
	})
}

type BatchBillingLineView struct {
	CustomId     string `json:"custom_id"`
	Status       string `json:"status"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CachedTokens int64  `json:"cached_tokens"`
	FinalQuota   int    `json:"quota"`
}

func GetBatchBillingLines(jobID string, offset, limit int) ([]BatchBillingLineView, error) {
	var result []BatchBillingLineView
	err := DB.Model(&BatchJobLine{}).Select("custom_id", "status", "input_tokens", "output_tokens", "cached_tokens", "final_quota").Where("job_id = ?", jobID).Order("id").Offset(offset).Limit(limit).Scan(&result).Error
	return result, err
}
