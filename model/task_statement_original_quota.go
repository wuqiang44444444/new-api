package model

import "github.com/QuantumNous/new-api/pkg/billingexpr"

// Read-only selection of accepted evidence for logs and delayed delivery.
func (task *Task) AcceptedBillingCalculation() *billingexpr.Calculation {
	if async := task.PrivateData.AsyncBilling; async != nil && async.CalculationSource != "initial" && async.Calculation != nil {
		return async.Calculation
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		if bc.SettlementCalculation != nil {
			return bc.SettlementCalculation
		}
		return bc.InitialCalculation
	}
	return nil
}

// Task events book a reservation, a difference against it, or a full refund.
// Use the corresponding recorded pre-discount amounts from those same events.
func TaskStatementOriginalQuota(task *Task, event TaskBillingDelivery, logType int) *int {
	billing := task.PrivateData.BillingContext
	async := task.PrivateData.AsyncBilling
	if billing == nil || async == nil {
		return nil
	}
	settled := task.AcceptedBillingCalculation()
	if event.Event == "complete" {
		return taskStatementCalculationOriginal(task, settled, event.AfterQuota, false)
	}
	if event.Event == "customer_refund" && async.State == TaskBillingStateSettled {
		if event.AfterQuota == 0 &&
			(async.CalculationSource == "initial" || async.Calculation != nil || billing.SettlementCalculation != nil) {
			return taskStatementCalculationOriginal(task, settled, event.BeforeQuota, async.CalculationSource == "initial")
		}
		return nil
	}
	initial := billing.InitialCalculation
	switch event.Event {
	case "create":
		return taskStatementCalculationOriginal(task, initial, event.AfterQuota, true)
	case "adjustment":
		before := taskStatementCalculationOriginal(task, initial, event.BeforeQuota, true)
		after := taskStatementCalculationOriginal(task, settled, event.AfterQuota, false)
		if before == nil || after == nil {
			return nil
		}
		original := *after - *before
		if logType == LogTypeRefund {
			original = -original
		}
		if original < 0 {
			return nil
		}
		return &original
	case "refund", "customer_refund":
		if event.AfterQuota != 0 {
			return nil
		}
		return taskStatementCalculationOriginal(task, initial, event.BeforeQuota, true)
	default:
		return nil
	}
}
