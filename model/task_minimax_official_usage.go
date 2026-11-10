package model

import (
	"errors"
	"fmt"
	"maps"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/minimaxlink"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
)

var ErrMiniMaxOfficialUsageMissing = errors.New("required MiniMax official usage is missing")

func (t *Task) IsMiniMaxV2Task() bool {
	return t != nil && t.Platform == constant.TaskPlatform(MiniMaxLinkTaskPlatform()) &&
		constant.IsMiniMaxVideoV2(string(t.PrivateData.VideoUpstreamProtocol))
}

// ObserveMiniMaxOfficialUsage selects the accepted measurements using the
// protocol bounds and the frozen estimate. A missing amount stays missing.
func (t *Task) ObserveMiniMaxOfficialUsage(observed map[string]any) {
	async := t.PrivateData.AsyncBilling
	if !t.IsMiniMaxV2Task() || t.Status != TaskStatusSuccess || async == nil || async.TieredSnapshot == nil {
		return
	}
	accepted := make(map[string]float64, len(observed))
	for field, raw := range observed {
		amount, valid := raw.(float64)
		if valid && validMiniMaxUsageWithinBudget(async, field, amount) {
			accepted[field] = amount
		}
	}
	async.OfficialUsage = accepted
}

func validMiniMaxUsageWithinBudget(async *TaskAsyncBillingContext, field string, amount float64) bool {
	if async.TieredSnapshot == nil {
		return false
	}
	budget, hasBudget := async.TieredSnapshot.UsageFacts[field].(float64)
	return hasBudget && minimaxlink.ValidOfficialMeasurement(field, budget) &&
		minimaxlink.ValidOfficialMeasurement(field, amount) && amount <= budget
}

// acceptMiniMaxUsageStatement runs under the same Task row lock as provider
// observations. It accepts only new evidence within the frozen budget.
func (t *Task) acceptMiniMaxUsageStatement(measurements map[string]*float64, reference string) (bool, error) {
	if !t.IsMiniMaxV2Task() || len(measurements) == 0 {
		return false, errors.New("verified MiniMax media usage is required")
	}
	async := t.PrivateData.AsyncBilling
	changed := false
	for field, amount := range measurements {
		if amount == nil || !validMiniMaxUsageWithinBudget(async, field, *amount) {
			return false, errors.New("invalid MiniMax media measurement for the frozen request")
		}
		if accepted, exists := async.OfficialUsage[field]; exists {
			if accepted != *amount {
				return false, errors.New("accepted usage cannot be replaced")
			}
			continue
		}
		if async.State != TaskBillingStateAwaitingUsage {
			return false, errors.New("task is not awaiting usage")
		}
		if async.OfficialUsage == nil {
			async.OfficialUsage = make(map[string]float64)
		}
		if async.OfficialUsageStatements == nil {
			async.OfficialUsageStatements = make(map[string]string)
		}
		async.OfficialUsage[field] = *amount
		async.OfficialUsageStatements[field] = reference
		changed = true
	}
	if !changed {
		return false, nil
	}
	if _, err := t.MiniMaxOfficialSettlementUsage(); err != nil {
		if errors.Is(err, ErrMiniMaxOfficialUsageMissing) {
			return true, nil
		}
		return false, err
	}
	async.State, async.Error, async.Attempts, async.NextRetryAt = TaskBillingStatePending, "", 0, 0
	t.BillingState = TaskBillingStatePending
	t.UsageCheckNextAt, t.UsageCheckManualAt = 0, 0
	return true, nil
}

func mergeMiniMaxOfficialUsage(next, proposed *TaskAsyncBillingContext) {
	if len(proposed.OfficialUsage) == 0 {
		return
	}
	next.OfficialUsage = maps.Clone(next.OfficialUsage)
	if next.OfficialUsage == nil {
		next.OfficialUsage = make(map[string]float64, len(proposed.OfficialUsage))
	}
	for field, amount := range proposed.OfficialUsage {
		if previous, accepted := next.OfficialUsage[field]; !accepted {
			next.OfficialUsage[field] = amount
		} else if previous != amount {
			if next.OfficialUsageConflict == nil {
				next.OfficialUsageConflict = make(map[string]float64)
			}
			if _, recorded := next.OfficialUsageConflict[field]; !recorded {
				next.OfficialUsageConflict[field] = amount
			}
		}
	}
}

// MiniMaxOfficialSettlementUsage supplies the measured facts required by the
// frozen expression. Only absence of media in the frozen request proves zero
// without a report. This same check gates both evaluation and funds writes.
func (t *Task) MiniMaxOfficialSettlementUsage() (map[string]any, error) {
	async := t.PrivateData.AsyncBilling
	if async == nil || async.TieredSnapshot == nil {
		return nil, errors.New("missing MiniMax official billing snapshot")
	}
	snapshot := async.TieredSnapshot
	required, err := seedancebilling.ReferencedUsageFields(snapshot.ExprString, minimaxlink.OfficialUsageFields())
	if err != nil {
		return nil, err
	}
	facts := make(map[string]any, len(required))
	for field := range required {
		if field == "resolution" {
			resolution, ok := snapshot.UsageFacts[field].(string)
			if !ok || (resolution != "768p" && resolution != "2k") {
				return nil, errors.New("missing frozen MiniMax official resolution")
			}
			facts[field] = resolution
			continue
		}
		budget, hasBudget := snapshot.UsageFacts[field].(float64)
		if !hasBudget || !minimaxlink.ValidOfficialMeasurement(field, budget) {
			return nil, fmt.Errorf("invalid frozen MiniMax official budget for %s", field)
		}
		amount, reported := async.OfficialUsage[field]
		if budget != 0 && !reported {
			return nil, ErrMiniMaxOfficialUsageMissing
		}
		if !minimaxlink.ValidOfficialMeasurement(field, amount) || amount > budget {
			return nil, fmt.Errorf("invalid MiniMax official measurement for %s", field)
		}
		facts[field] = amount
	}
	return facts, nil
}
