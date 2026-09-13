package model

import (
	"fmt"
	"maps"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"gorm.io/gorm"
)

// TaskUsageDiscrepancy is the first conflicting observation, not a replacement
// for accepted usage. It contains no provider payload or media references.
type TaskUsageDiscrepancy struct {
	Reported bool           `json:"reported"`
	Tokens   int            `json:"tokens"`
	Source   string         `json:"source,omitempty"`
	Evidence map[string]int `json:"evidence,omitempty"`
}

// HasSeedanceBillingFacts uses the frozen code protocol, never model names or current channel settings.
func (t *Task) HasSeedanceBillingFacts() bool {
	return t != nil && t.PrivateData.AsyncBilling != nil && t.PrivateData.VideoUpstreamProtocol.IsValid()
}

// mergeSeedanceBillingFacts runs under the task row lock. Observation writes
// can contribute first usage; only billing writes can establish a target.
func mergeSeedanceBillingFacts(stored, proposed *TaskAsyncBillingContext, observation bool) (*TaskAsyncBillingContext, error) {
	if proposed.TargetQuota != nil && *proposed.TargetQuota < 0 {
		return nil, fmt.Errorf("negative task billing target")
	}
	next := *stored
	if stored.ActualUsageReported {
		if (proposed.ActualUsageReported != stored.ActualUsageReported || proposed.ActualTokens != stored.ActualTokens || proposed.ActualUsageSource != stored.ActualUsageSource || !maps.Equal(proposed.ActualUsageEvidence, stored.ActualUsageEvidence)) && observation && next.UsageDiscrepancy == nil {
			next.UsageDiscrepancy = &TaskUsageDiscrepancy{Reported: proposed.ActualUsageReported, Tokens: proposed.ActualTokens, Source: proposed.ActualUsageSource, Evidence: proposed.ActualUsageEvidence}
		}
	} else if proposed.ActualUsageReported {
		next.ActualTokens = proposed.ActualTokens
		next.ActualUsageReported = true
		next.ActualUsageSource = proposed.ActualUsageSource
		next.ActualUsageEvidence = proposed.ActualUsageEvidence
		next.ProviderBillingEvidence = proposed.ProviderBillingEvidence
		if next.State == TaskBillingStateAwaitingUsage {
			next.State, next.Error, next.NextRetryAt, next.Attempts = TaskBillingStatePending, "", 0, 0
		}
	} else if len(next.ActualUsageEvidence) == 0 {
		next.ActualUsageEvidence = proposed.ActualUsageEvidence
	}
	if observation || stored.State == TaskBillingStateSettled {
		return &next, nil
	}
	// A debt/failed target is already a funding instruction. A later missing or
	// contradictory observation cannot replace it or turn it back into waiting.
	if stored.TargetQuota != nil && (proposed.TargetQuota == nil || *proposed.TargetQuota != *stored.TargetQuota) {
		return &next, nil
	}
	if proposed.State == TaskBillingStateAwaitingUsage && next.ActualUsageReported && proposed.TargetQuota == nil {
		return &next, nil
	}
	next.State, next.Error = proposed.State, proposed.Error
	next.Attempts, next.NextRetryAt = proposed.Attempts, proposed.NextRetryAt
	next.Operation, next.Reason = proposed.Operation, proposed.Reason
	next.QuotaClamp = proposed.QuotaClamp
	if next.TargetQuota == nil {
		next.TargetQuota = proposed.TargetQuota
	}
	if next.TargetQuota != nil && next.State == TaskBillingStateAwaitingUsage {
		next.State, next.Error, next.NextRetryAt, next.Attempts = TaskBillingStatePending, "", 0, 0
	}
	return &next, nil
}

// updateSeedanceBillingFacts never writes the caller's quota or connection.
// Only ApplyTaskBillingTarget may change quota together with actual funding.
func (t *Task) updateSeedanceBillingFacts() error {
	var saved Task
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&saved, t.ID).Error; err != nil {
			return err
		}
		if !saved.HasSeedanceBillingFacts() {
			return fmt.Errorf("missing frozen video billing facts")
		}
		next, err := mergeSeedanceBillingFacts(saved.PrivateData.AsyncBilling, t.PrivateData.AsyncBilling, false)
		if err != nil {
			return err
		}
		saved.PrivateData.AsyncBilling = next
		saved.BillingState = next.State
		return tx.Model(&saved).Updates(map[string]any{"private_data": saved.PrivateData, "billing_state": saved.BillingState}).Error
	})
	if err == nil {
		*t = saved
	}
	return err
}

func (t *Task) updateSeedanceObservation(fromStatus TaskStatus) (bool, error) {
	var saved Task
	accepted := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var stored Task
		if err := lockForUpdate(tx).First(&stored, t.ID).Error; err != nil {
			return err
		}
		saved = stored
		if stored.Status != fromStatus {
			return nil
		}
		if stored.Status.IsTerminal() && stored.Status != t.Status {
			return nil
		}
		if !stored.HasSeedanceBillingFacts() {
			return fmt.Errorf("missing frozen video billing facts")
		}
		next, err := mergeSeedanceBillingFacts(stored.PrivateData.AsyncBilling, t.PrivateData.AsyncBilling, true)
		if err != nil {
			return err
		}
		saved.Status, saved.Progress = t.Status, t.Progress
		saved.StartTime, saved.FinishTime, saved.FailReason = t.StartTime, t.FinishTime, t.FailReason
		saved.Data = t.Data
		// Keep accepted normalized usage in the public payload as well as the
		// billing evidence; other result fields may be refreshed independently.
		if stored.PrivateData.AsyncBilling.ActualUsageReported {
			var previous, current map[string]any
			if len(stored.Data) > 0 {
				if err := common.Unmarshal(stored.Data, &previous); err != nil {
					return err
				}
			}
			if len(saved.Data) > 0 {
				if err := common.Unmarshal(saved.Data, &current); err != nil {
					return err
				}
			}
			if current == nil {
				current = map[string]any{}
			}
			if usage, ok := previous["usage"]; ok {
				current["usage"] = usage
			} else {
				delete(current, "usage")
			}
			saved.Data, err = common.Marshal(current)
			if err != nil {
				return err
			}
		}
		saved.PrivateData.ResultURL = t.PrivateData.ResultURL
		saved.PrivateData.PollFailures = t.PrivateData.PollFailures
		saved.PrivateData.AsyncBilling = next
		saved.BillingState = next.State
		if err := tx.Model(&saved).Select("status", "progress", "start_time", "finish_time", "fail_reason", "data", "private_data", "billing_state").Updates(&saved).Error; err != nil {
			return err
		}
		accepted = true
		return nil
	})
	if err == nil {
		*t = saved
	}
	return accepted, err
}

// enforceSeedanceBillingTarget is called while holding the funding task lock.
func enforceSeedanceBillingTarget(task *Task, target int) error {
	if !task.HasSeedanceBillingFacts() {
		return nil
	}
	async := task.PrivateData.AsyncBilling
	if async.TargetQuota != nil && *async.TargetQuota != target {
		return fmt.Errorf("task billing target conflicts with frozen target")
	}
	if !task.Status.IsTerminal() {
		return fmt.Errorf("cannot settle a nonterminal task")
	}
	// 与提交、结算、补查共用 §5.5 的实测依赖规则：u() 表达式只等 tokens 实测，
	// 纯冻结条件表达式不误等待；旧 c/_task 表达式保持通用用量依赖。
	if task.Status == TaskStatusSuccess && async.TieredSnapshot != nil &&
		seedancebilling.RequiresMeasuredTaskUsage(async.TieredSnapshot) && !async.ActualUsageReported {
		return fmt.Errorf("cannot settle without required provider usage")
	}
	return nil
}

func (t *Task) projectSeedanceAcceptedUsage(result *dto.ModelArkVideoTask) {
	if !t.HasSeedanceBillingFacts() {
		return
	}
	async := t.PrivateData.AsyncBilling
	if !async.ActualUsageReported {
		return
	}
	if result.Usage == nil {
		result.Usage = &dto.ModelArkVideoTaskUsage{}
	}
	result.Usage.CompletionTokens = async.ActualTokens
}
