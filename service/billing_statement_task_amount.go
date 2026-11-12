package service

import (
	"github.com/QuantumNous/new-api/model"
)

// Task events book a reservation, a difference against it, or a full refund.
// Use the corresponding recorded pre-discount amounts from those same events.
func appendTaskStatementOriginalQuota(other *model.LogOther, task *model.Task, event model.TaskBillingDelivery, logType int) {
	billing := task.PrivateData.BillingContext
	async := task.PrivateData.AsyncBilling
	if billing == nil || async == nil {
		return
	}
	settled := taskAcceptedCalculation(task)
	if event.Event == "complete" {
		if settled != nil && settled.Quota == event.AfterQuota && settled.OriginalQuota != nil {
			other.SetAdmin(model.StatementOriginalQuotaLogKey, *settled.OriginalQuota)
		}
		return
	}
	if event.Event == "customer_refund" && async.State == model.TaskBillingStateSettled {
		if event.AfterQuota == 0 && settled != nil && settled.Quota == event.BeforeQuota && settled.OriginalQuota != nil &&
			(async.CalculationSource == "initial" || async.Calculation != nil || billing.SettlementCalculation != nil) {
			other.SetAdmin(model.StatementOriginalQuotaLogKey, *settled.OriginalQuota)
		}
		return
	}
	initial := billing.InitialCalculation
	if initial == nil || initial.OriginalQuota == nil {
		return
	}
	var original int
	switch event.Event {
	case "create":
		if initial.Quota != event.AfterQuota {
			return
		}
		original = *initial.OriginalQuota
	case "adjustment":
		if initial.Quota != event.BeforeQuota || settled == nil ||
			settled.Quota != event.AfterQuota || settled.OriginalQuota == nil {
			return
		}
		original = *settled.OriginalQuota - *initial.OriginalQuota
		if logType == model.LogTypeRefund {
			original = -original
		}
		if original < 0 {
			return
		}
	case "refund", "customer_refund":
		if initial.Quota != event.BeforeQuota || event.AfterQuota != 0 {
			return
		}
		original = *initial.OriginalQuota
	default:
		return
	}
	other.SetAdmin(model.StatementOriginalQuotaLogKey, original)
}
