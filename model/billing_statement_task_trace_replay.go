package model

import (
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/shopspring/decimal"
)

// Older task traces saved the output of quota conversion and all subsequent
// arithmetic. Verify that exact chain against the frozen factors and delivery.
func taskStatementCalculationOriginal(task *Task, calc *billingexpr.Calculation, booked int, initial bool) *int {
	if calc == nil || booked < 0 || calc.Quota != booked {
		return nil
	}
	if calc.OriginalQuota != nil {
		return calc.OriginalQuota
	}
	billing, async := task.PrivateData.BillingContext, task.PrivateData.AsyncBilling
	if calc.Version != 1 || async.QuotaClamp != nil {
		return nil
	}
	snapshot := async.TieredSnapshot
	if snapshot == nil {
		snapshot = billing.TieredSnapshot
	}
	contract := decimal.NewFromInt(1)
	if billing.ContractFact != nil {
		contract = billing.ContractFact.RatioDecimal()
		if !contract.IsPositive() {
			return nil
		}
	}
	trace := verifiedTaskStatementTrace(calc, snapshot, contract, booked, initial)
	if trace == nil {
		return nil
	}
	return &trace.original
}

type taskStatementTrace struct {
	beforeGroup float64
	original    int
}

func verifiedTaskStatementTrace(calc *billingexpr.Calculation, snapshot *billingexpr.BillingSnapshot, contract decimal.Decimal, booked int, initial bool) *taskStatementTrace {
	if calc == nil || calc.Version != 1 || booked < 0 || calc.Quota != booked ||
		snapshot == nil || snapshot.QuotaPerUnit <= 0 || snapshot.GroupRatio < 0 ||
		!finiteStatementNumber(snapshot.QuotaPerUnit) || !finiteStatementNumber(snapshot.GroupRatio) {
		return nil
	}
	var base float64
	conversions := 0
	for _, step := range calc.Steps {
		if step.Op != "quota_conversion" {
			continue
		}
		conversions++
		if step.Unit != "quota" || len(step.Inputs) != 3 || !equalTaskStatementDecimal(step.Inputs[1], decimal.NewFromFloat(snapshot.QuotaPerUnit)) {
			return nil
		}
		cost, costErr := strconv.ParseFloat(step.Inputs[0], 64)
		value, valueErr := strconv.ParseFloat(step.Result, 64)
		divisor := int64(1_000_000)
		if snapshot.TaskUsageBilling {
			divisor = 1
		}
		if costErr != nil || valueErr != nil || cost < 0 || !finiteStatementNumber(cost) ||
			!finiteStatementNumber(value) || !equalTaskStatementDecimal(step.Inputs[2], decimal.NewFromInt(divisor)) ||
			value != cost/float64(divisor)*snapshot.QuotaPerUnit {
			return nil
		}
		base = value
	}
	original, err := common.QuotaRoundStrict(base)
	if conversions != 1 || err != nil || (initial && base != snapshot.EstimatedQuotaBeforeGroup) ||
		!finiteStatementNumber(base*snapshot.GroupRatio) {
		return nil
	}

	group := decimal.NewFromFloat(snapshot.GroupRatio)
	baseDecimal := decimal.NewFromFloat(base)
	groupedFloat := decimal.NewFromFloat(base * snapshot.GroupRatio)
	groupedDecimal := baseDecimal.Mul(group)
	var net decimal.Decimal
	var haveGroup, haveContract, verified bool
	for _, step := range calc.Steps {
		switch step.Op {
		case "quota_conversion":
			continue
		case "group_ratio":
			if step.Unit != "quota" || len(step.Inputs) != 2 ||
				!equalTaskStatementDecimal(step.Inputs[0], baseDecimal) || !equalTaskStatementDecimal(step.Inputs[1], group) {
				return nil
			}
			amount, ok := taskStatementDecimal(step.Result)
			if !ok || (!amount.Equal(groupedFloat) && !amount.Equal(groupedDecimal)) {
				return nil
			}
			net, haveGroup, haveContract, verified = amount, true, contract.Equal(decimal.NewFromInt(1)), false
		case "contract_ratio":
			if !haveGroup || step.Unit != "quota" || len(step.Inputs) != 2 || !equalTaskStatementDecimal(step.Inputs[1], contract) {
				return nil
			}
			before, ok := taskStatementDecimal(step.Inputs[0])
			if !ok || (!before.Equal(groupedFloat) && !before.Equal(groupedDecimal)) {
				return nil
			}
			net = before.Mul(contract)
			if !equalTaskStatementDecimal(step.Result, net) {
				return nil
			}
			haveContract, verified = true, false
		case "round":
			if !haveGroup || step.Unit != "quota" || len(step.Inputs) != 1 {
				return nil
			}
			input, inputErr := strconv.ParseFloat(step.Inputs[0], 64)
			recorded, recordErr := strconv.Atoi(step.Result)
			amount := net.InexactFloat64()
			rounded, roundErr := common.QuotaRoundStrict(amount)
			if inputErr != nil || recordErr != nil || roundErr != nil || input != amount || recorded != rounded {
				return nil
			}
			verified = haveContract && rounded == booked
		case "truncate", "other_ratios":
			return nil
		default:
			if haveGroup {
				return nil
			}
		}
	}
	if !verified {
		return nil
	}
	return &taskStatementTrace{beforeGroup: base, original: original}
}

func taskStatementDecimal(raw string) (decimal.Decimal, bool) {
	if len(raw) > 64 {
		return decimal.Zero, false
	}
	value, err := decimal.NewFromString(raw)
	return value, err == nil && value.Exponent() >= -512 && value.Exponent() <= 512
}

func equalTaskStatementDecimal(raw string, expected decimal.Decimal) bool {
	value, ok := taskStatementDecimal(raw)
	return ok && value.Equal(expected)
}

func finiteStatementNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
