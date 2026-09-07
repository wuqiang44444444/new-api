package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

const (
	TaskUsageCheckMaxAttempts         = 12
	TaskUsageCheckWindowSeconds int64 = 24 * 60 * 60
)

// TaskUsageRecovery stores scheduling and operator audit facts, never money.
type TaskUsageRecovery struct {
	UsageCheckStartedAt   int64  `json:"-" gorm:"bigint"`
	UsageCheckNextAt      int64  `json:"-" gorm:"bigint;index"`
	UsageCheckAttempts    int    `json:"-"`
	UsageCheckManualAt    int64  `json:"-" gorm:"bigint"`
	UsageCheckLastError   string `json:"-" gorm:"type:varchar(64)"`
	UsageReviewAt         int64  `json:"-" gorm:"bigint;index"`
	UsageReviewOperatorID int    `json:"-"`
	UsageReviewReference  string `json:"-" gorm:"type:varchar(256)"`
}

func dueTaskUsageChecks(db *gorm.DB, now int64) *gorm.DB {
	return db.Model(&Task{}).Where("billing_state = ? AND status = ?", TaskBillingStateAwaitingUsage, TaskStatusSuccess).
		Where("(usage_review_at IS NULL OR usage_review_at = 0) AND (usage_check_next_at IS NULL OR usage_check_next_at <= ?)", now)
}
func HasDueTaskUsageChecks(now int64) bool {
	var id int64
	return dueTaskUsageChecks(DB, now).Limit(1).Pluck("id", &id).Error == nil && id != 0
}
func DueTaskUsageCheckIDs(now int64, limit int) ([]int64, error) {
	var ids []int64
	err := dueTaskUsageChecks(DB, now).Order("usage_check_next_at, id").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// ClaimTaskUsageCheck commits the next attempt before I/O. A second runner sees
// the future timestamp. A crashed runner consumes one attempt, not an unbounded retry.
func ClaimTaskUsageCheck(id, now int64) (*Task, bool, error) {
	var task Task
	claimed, review := false, false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&task, id).Error; err != nil {
			return err
		}
		if !task.HasSeedanceBillingFacts() || task.Status != TaskStatusSuccess || task.BillingState != TaskBillingStateAwaitingUsage || task.UsageReviewAt > 0 || task.UsageCheckNextAt > now {
			return nil
		}
		if task.UsageCheckStartedAt == 0 {
			task.UsageCheckStartedAt = task.FinishTime
			if task.UsageCheckStartedAt <= 0 {
				task.UsageCheckStartedAt = now
			}
		}
		deadline := task.UsageCheckStartedAt + TaskUsageCheckWindowSeconds
		if task.UsageCheckManualAt == 0 && (now >= deadline || task.UsageCheckAttempts >= TaskUsageCheckMaxAttempts) {
			task.UsageReviewAt = now
			task.UsageCheckNextAt = 0
			review = true
		} else {
			task.UsageCheckAttempts++
			delay := min(int64(60)<<min(task.UsageCheckAttempts-1, 9), int64(6*60*60))
			if task.UsageCheckManualAt > 0 {
				task.UsageCheckNextAt = now + 60
				task.UsageCheckManualAt = 0
			} else {
				task.UsageCheckNextAt = min(now+delay, deadline)
			}
			task.UsageCheckLastError = "observation_incomplete"
			claimed = true
		}
		return tx.Model(&task).Updates(map[string]any{
			"usage_check_started_at": task.UsageCheckStartedAt, "usage_check_next_at": task.UsageCheckNextAt,
			"usage_check_attempts": task.UsageCheckAttempts, "usage_check_manual_at": task.UsageCheckManualAt, "usage_review_at": task.UsageReviewAt, "usage_check_last_error": task.UsageCheckLastError,
		}).Error
	})
	if err != nil {
		return nil, false, err
	}
	if !claimed && !review {
		return nil, false, nil
	}
	return &task, review, nil
}

func FinishTaskUsageCheck(task *Task, observationFailed bool) error {
	code := "usage_missing"
	if observationFailed {
		code = "query_failed"
	}
	// Match the claimed generation; a late worker cannot overwrite a newer check.
	return DB.Model(&Task{}).Where("id = ? AND billing_state = ? AND usage_check_attempts = ? AND usage_check_next_at = ?",
		task.ID, TaskBillingStateAwaitingUsage, task.UsageCheckAttempts, task.UsageCheckNextAt).
		Update("usage_check_last_error", code).Error
}

type TaskUsageRecoveryItem struct {
	TaskID    string `json:"task_id"`
	UserID    int    `json:"user_id"`
	AppID     int    `json:"app_id"`
	Model     string `json:"model"`
	HeldQuota int    `json:"held_quota"`
	StartedAt int64  `json:"started_at"`
	NextAt    int64  `json:"next_at"`
	Attempts  int    `json:"attempts"`
	ReviewAt  int64  `json:"review_at"`
	LastError string `json:"last_error"`
}

func ListTaskUsageRecovery(reviewOnly bool, offset, limit int) ([]TaskUsageRecoveryItem, error) {
	if offset < 0 || limit < 1 || limit > 100 {
		return nil, errors.New("invalid pagination")
	}
	q := DB.Where("billing_state = ? AND status = ?", TaskBillingStateAwaitingUsage, TaskStatusSuccess)
	if reviewOnly {
		q = q.Where("usage_review_at > ?", 0)
	}
	var tasks []Task
	if err := q.Order("id").Offset(offset).Limit(limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	out := make([]TaskUsageRecoveryItem, 0, len(tasks))
	for _, t := range tasks {
		if !t.HasSeedanceBillingFacts() {
			continue
		}
		out = append(out, TaskUsageRecoveryItem{t.TaskID, t.UserId, t.AppID, t.Properties.OriginModelName, t.Quota, t.UsageCheckStartedAt, t.UsageCheckNextAt, t.UsageCheckAttempts, t.UsageReviewAt, t.UsageCheckLastError})
	}
	return out, nil
}

// ReviewTaskUsage never changes quota. Verified usage becomes the same immutable
// first observation used by polling; settlement remains ApplyTaskBillingTarget.
func ReviewTaskUsage(taskID string, operatorID int, reference string, tokens *int) (*Task, error) {
	reference = strings.TrimSpace(reference)
	if operatorID <= 0 || reference == "" || len(reference) > 256 || len(taskID) > 64 || (tokens != nil && (*tokens < 0 || int64(*tokens) > int64(common.MaxQuota))) {
		return nil, errors.New("operator, bounded usage, and statement or incident reference are required")
	}
	reviewTime := GetDBTimestamp()
	var task Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
			return err
		}
		if !task.HasSeedanceBillingFacts() || task.Status != TaskStatusSuccess {
			return errors.New("task is not eligible for usage review")
		}
		async := task.PrivateData.AsyncBilling
		if tokens != nil && *tokens == 0 && task.PrivateData.VideoUpstreamProtocol == dto.VideoUpstreamProtocolFunCloudSeedance {
			return errors.New("FunCloud V2 requires positive completionTokens")
		}
		if async.ActualUsageReported {
			if tokens != nil && *tokens == async.ActualTokens {
				return nil
			}
			return errors.New("accepted usage cannot be replaced")
		}
		if async.State != TaskBillingStateAwaitingUsage {
			return errors.New("task is not awaiting usage")
		}
		task.UsageReviewOperatorID = operatorID
		task.UsageReviewReference = reference
		if tokens == nil {
			// Explicit operator action permits one more GET, not another retry window.
			task.UsageCheckNextAt = reviewTime
			task.UsageReviewAt = 0
			task.UsageCheckManualAt = task.UsageCheckNextAt
		} else {
			async.ActualTokens = *tokens
			async.ActualUsageReported = true
			async.ActualUsageSource = "operator_verified_statement"
			async.ActualUsageEvidence = map[string]int{"statement.completion_tokens": *tokens}
			async.State, async.Error, async.Attempts, async.NextRetryAt = TaskBillingStatePending, "", 0, 0
			task.BillingState = TaskBillingStatePending
			var payload map[string]any
			if len(task.Data) > 0 {
				if err := common.Unmarshal(task.Data, &payload); err != nil {
					return fmt.Errorf("invalid stored task result: %w", err)
				}
			}
			if payload == nil {
				payload = map[string]any{}
			}
			payload["usage"] = &dto.ModelArkVideoTaskUsage{CompletionTokens: *tokens, TotalTokens: *tokens}
			var err error
			task.Data, err = common.Marshal(payload)
			if err != nil {
				return err
			}
			task.UsageCheckNextAt = 0
		}
		return tx.Model(&task).Updates(map[string]any{
			"private_data": task.PrivateData, "billing_state": task.BillingState, "data": task.Data,
			"usage_review_operator_id": task.UsageReviewOperatorID, "usage_review_reference": task.UsageReviewReference,
			"usage_check_next_at": task.UsageCheckNextAt, "usage_review_at": task.UsageReviewAt,
			"usage_check_manual_at": task.UsageCheckManualAt,
		}).Error
	})
	return &task, err
}
