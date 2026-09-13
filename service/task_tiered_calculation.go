package service

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/shopspring/decimal"
)

// ComputeTaskTieredBilling evaluates only frozen task facts. Settlement, log
// projection and offline repair share the same input, unit and discount rules.
// It never persists facts or performs a funding operation.
func ComputeTaskTieredBilling(task *model.Task) (billingexpr.TieredResult, billingexpr.RequestInput, error) {
	var result billingexpr.TieredResult
	var input billingexpr.RequestInput
	async := task.PrivateData.AsyncBilling
	if async == nil || async.TieredSnapshot == nil {
		return result, input, fmt.Errorf("missing frozen task expression")
	}
	snap := async.TieredSnapshot
	if err := billingexpr.UnknownIdentifier(snap.ExprString); err != nil {
		return result, input, err
	}
	if task.HasSeedanceBillingFacts() && !async.ActualUsageReported && seedancebilling.RequiresMeasuredTaskUsage(snap) {
		return result, input, fmt.Errorf("required actual task usage is missing")
	}
	if async.BillingProbe != nil {
		input = *async.BillingProbe
	}
	if snap.TaskUsageBilling {
		facts, err := seedancebilling.ControlledFacts(input.Body, async.ActualTokens)
		if err != nil {
			return result, input, err
		}
		input.Usage = facts
	}
	var err error
	result, err = billingexpr.ComputeTieredQuotaWithRequest(snap, billingexpr.TokenParams{C: float64(async.ActualTokens)}, input)
	if err != nil {
		return result, input, err
	}
	if math.IsNaN(result.ActualQuotaBeforeGroup) || math.IsInf(result.ActualQuotaBeforeGroup, 0) || result.ActualQuotaBeforeGroup < 0 || result.ActualQuotaAfterGroup < 0 {
		return result, input, fmt.Errorf("frozen task price must be finite and non-negative")
	}
	if bc := task.PrivateData.BillingContext; bc != nil && bc.ContractFact != nil {
		amount := decimal.NewFromFloat(result.ActualQuotaBeforeGroup).Mul(decimal.NewFromFloat(snap.GroupRatio))
		amount, err = ApplyCustomerContractRatio(amount, bc.ContractFact)
		if err != nil {
			return result, input, err
		}
		result.ActualQuotaAfterGroup, result.Clamp = common.QuotaRoundChecked(amount.InexactFloat64())
	}
	return result, input, nil
}

// appendTaskSettlementExpressionFacts never presents a submission budget as
// completed usage. The original BillingSnapshot remains immutable.
func appendTaskSettlementExpressionFacts(task *model.Task, other *model.LogOther) {
	async := task.PrivateData.AsyncBilling
	if async == nil || async.TieredSnapshot == nil || async.Operation != "settle" || async.State != model.TaskBillingStateSettled {
		return
	}
	result, input, err := ComputeTaskTieredBilling(task)
	if err != nil {
		other.SetAdmin("task_expression_projection_unavailable", true)
		return
	}
	other.SetPublic("matched_tier", result.MatchedTier)
	other.SetPublic("request_rules", result.RequestRules)
	if len(input.Usage) > 0 {
		if !async.ActualUsageReported {
			delete(input.Usage, seedancebilling.TokenUsageKey)
		}
		other.SetPublic("usage_facts", input.Usage)
	}
}
