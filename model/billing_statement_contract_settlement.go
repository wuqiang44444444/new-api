package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func attachTaskContractSettlement(task *Task, parsed *parsedBillingReconciliationLog, event string, before, after, logType int) {
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.ContractFact == nil ||
		(billing.ContractFact.UserId != 0 && billing.ContractFact.UserId != task.UserId) ||
		(billing.ContractFact.PublicModel != "" && billing.ContractFact.PublicModel != parsed.customerModel) {
		return
	}
	snapshot := billing.TieredSnapshot
	if async := task.PrivateData.AsyncBilling; async != nil {
		if async.QuotaClamp != nil {
			return
		}
		if async.TieredSnapshot != nil {
			snapshot = async.TieredSnapshot
		}
	}
	contract := billing.ContractFact.RatioDecimal()
	trace := taskFinalContractTrace(task.AcceptedBillingCalculation(), snapshot, after)
	if trace == nil || !contract.IsPositive() {
		return
	}
	expected, err := common.QuotaRoundStrict(decimal.NewFromFloat(trace.beforeGroup).
		Mul(decimal.NewFromFloat(snapshot.GroupRatio)).Mul(contract).InexactFloat64())
	if err != nil || expected == after {
		return
	}
	parsed.contractSettlement = &CustomerContractSettlement{
		ExpectedQuota: int64(expected), ActualQuota: int64(after), DeltaQuota: int64(after) - int64(expected),
	}
	if parsed.recordedOriginalQuota != nil || parsed.replayedOriginalQuota != nil {
		return
	}
	original := trace.original
	if event == "adjustment" {
		initial := verifiedTaskStatementTrace(billing.InitialCalculation, snapshot, contract, before, true)
		if initial == nil {
			return
		}
		original -= initial.original
		if logType == LogTypeRefund {
			original = -original
		}
	}
	if original >= 0 {
		amount := int64(original)
		parsed.replayedOriginalQuota = &amount
		parsed.originalReplayReason = ""
	}
}

// Read the applied factor from the final trace and check that its forward
// arithmetic reproduced the independently linked settlement. The frozen
// contract is applied separately to compute the amount that was due.
func taskFinalContractTrace(calc *billingexpr.Calculation, snapshot *billingexpr.BillingSnapshot, booked int) *taskStatementTrace {
	if calc == nil {
		return nil
	}
	actual := decimal.NewFromInt(1)
	for _, step := range calc.Steps {
		switch step.Op {
		case "group_ratio":
			actual = decimal.NewFromInt(1)
		case "contract_ratio":
			if len(step.Inputs) != 2 {
				return nil
			}
			ratio, ok := taskStatementDecimal(step.Inputs[1])
			if !ok || !ratio.IsPositive() || ratio.GreaterThan(decimal.NewFromInt(1)) {
				return nil
			}
			actual = ratio
		}
	}
	return verifiedTaskStatementTrace(calc, snapshot, actual, booked, false)
}

// Some older tasks emitted the initial charge and settlement difference
// without a durable delivery row. Require both scoped logs, exact timestamps
// and reconciliation to the frozen calculations before using this evidence.
func recoverLegacyTaskStatement(ctx context.Context, log billingReconciliationLog, parsed *parsedBillingReconciliationLog) error {
	event := parsed.taskBillingEvent
	if (event != "create" && event != "adjustment") ||
		parsed.contractDiscountRatio == nil ||
		(log.Type != LogTypeConsume && (event != "adjustment" || log.Type != LogTypeRefund)) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db := usageReportMainDB(ctx).WithContext(ctx)
	var task Task
	err := db.Where("task_id = ? AND user_id = ? AND app_id = ? AND channel_id = ? AND status = ?",
		parsed.taskID, log.UserId, log.TokenId, log.ChannelId, TaskStatusSuccess).Take(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	billing := task.PrivateData.BillingContext
	if billing == nil || billing.TieredSnapshot == nil || billing.ContractFact == nil ||
		billing.InitialCalculation == nil || billing.SettlementCalculation == nil || task.PrivateData.AsyncBilling != nil ||
		task.PrivateData.TokenId != log.TokenId || task.SubmitTime <= 0 || task.FinishTime < task.SubmitTime {
		return nil
	}
	customerModel := billing.OriginModelName
	if customerModel == "" {
		customerModel = task.Properties.OriginModelName
	}
	contract := billing.ContractFact.RatioDecimal()
	if customerModel != parsed.customerModel || !contract.IsPositive() ||
		(billing.ContractFact.UserId != 0 && billing.ContractFact.UserId != log.UserId) ||
		(billing.ContractFact.PublicModel != "" && billing.ContractFact.PublicModel != parsed.customerModel) ||
		!contract.Equal(decimal.NewFromFloat(*parsed.contractDiscountRatio)) {
		return nil
	}
	before, after := billing.InitialCalculation.Quota, billing.SettlementCalculation.Quota
	if before < 0 || after < 0 || task.Quota != after {
		return nil
	}
	var candidates []Log
	err = usageReportLogDB(ctx).WithContext(ctx).
		Select("user_id", "token_id", "channel_id", "model_name", "created_at", "type", "quota", "request_id", "other").
		Where("user_id = ? AND token_id = ? AND channel_id = ? AND model_name = ? AND type IN ? AND created_at IN ?",
			log.UserId, log.TokenId, log.ChannelId, log.ModelName, []int{LogTypeConsume, LogTypeRefund}, []int64{task.SubmitTime, task.FinishTime}).
		Limit(128).Find(&candidates).Error
	if err != nil || len(candidates) == 128 {
		return err
	}
	var creates, adjustments, current int
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.RequestId, "task-billing:") {
			continue
		}
		facts := loadCustomerExportFacts(candidate.Other)
		if facts.string("task_id") != parsed.taskID {
			continue
		}
		group, groupOK := facts.float("group_ratio")
		ratio, ratioOK := facts.float("contract_discount")
		id, idOK := billingBreakdownNonNegativeInt(facts.raw("contract_id"))
		version, versionOK := billingBreakdownNonNegativeInt(facts.raw("contract_version"))
		if !groupOK || group != billing.TieredSnapshot.GroupRatio || !ratioOK || !contract.Equal(decimal.NewFromFloat(ratio)) ||
			!idOK || id != int64(billing.ContractFact.ContractId) || !versionOK || version != int64(billing.ContractFact.ContractVersion) ||
			len(facts.other["admin_info"]) != 0 && !legacyTaskStatementLogUsable(facts) {
			return nil
		}
		switch candidateEvent := facts.string("task_billing_event"); candidateEvent {
		case "create":
			if candidate.CreatedAt != task.SubmitTime || candidate.Type != LogTypeConsume || candidate.Quota != before {
				return nil
			}
			creates++
		case "adjustment":
			held, heldOK := billingBreakdownNonNegativeInt(facts.raw("pre_consumed_quota"))
			final, finalOK := billingBreakdownNonNegativeInt(facts.raw("actual_quota"))
			amount := signedStatementEventQuota(int64(candidate.Quota), candidate.Type == LogTypeRefund)
			if candidate.CreatedAt != task.FinishTime || amount != int64(after)-int64(before) ||
				!heldOK || held != int64(before) || !finalOK || final != int64(after) {
				return nil
			}
			adjustments++
		default:
			return nil
		}
		if candidate.RequestId == log.RequestId && candidate.Type == log.Type && candidate.CreatedAt == log.CreatedAt && candidate.Quota == log.Quota {
			current++
		}
	}
	if creates != 1 || adjustments != 1 || current != 1 {
		return nil
	}
	initial := verifiedTaskStatementTrace(billing.InitialCalculation, billing.TieredSnapshot, contract, before, true)
	if initial == nil {
		return nil
	}
	if event == "create" {
		if parsed.recordedOriginalQuota == nil {
			original := int64(initial.original)
			parsed.replayedOriginalQuota = &original
			parsed.originalReplayReason = ""
		}
		return nil
	}
	attachTaskContractSettlement(&task, parsed, event, before, after, log.Type)
	return nil
}

func legacyTaskStatementLogUsable(facts customerExportFacts) bool {
	var admin map[string]any
	return common.Unmarshal(facts.other["admin_info"], &admin) == nil &&
		admin["quota_saturation"] == nil && admin["statement_funding_adjustment_failed"] != true
}
