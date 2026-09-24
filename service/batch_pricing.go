package service

import (
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	azurebatch "github.com/QuantumNous/new-api/relay/channel/azurebatch"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
)

// Batch billing evaluates the frozen per-model expression once per request
// line, then applies the frozen group ratio and contract discount to the line
// result. Tiers are judged per line; tokens are never pooled across lines.

// LoadBatchBillingForModel returns the batch expression configured for one
// model. A missing expression is a hard configuration error: batch pricing is
// never silently derived from the normal price.
func LoadBatchBillingForModel(modelName string) (string, error) {
	expr, ok := billing_setting.GetBatchBillingExpr(modelName)
	if !ok {
		return "", fmt.Errorf("model %q has no batch billing expression", modelName)
	}
	return expr, nil
}

// BatchLineUsage is the trusted per-line token observation normalized from
// the upstream result file. Cached tokens are the prompt-cache read subset.
type BatchLineUsage struct {
	CustomId     string
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64
}

// batchTokenParams normalizes one line's usage for the expression. OpenAI
// batch responses report prompt_tokens including cached reads, so the cached
// portion is subtracted from p only when the expression prices cache
// separately.
func batchTokenParams(expr string, usage BatchLineUsage) billingexpr.TokenParams {
	params := billingexpr.TokenParams{
		P:   float64(usage.InputTokens),
		C:   float64(usage.OutputTokens),
		Len: float64(usage.InputTokens),
	}
	used := billingexpr.UsedVars(expr)
	if used["cr"] {
		params.CR = float64(usage.CachedTokens)
		params.P = math.Max(0, params.P-float64(usage.CachedTokens))
	}
	return params
}

func batchLineModelCost(frozen *model.BatchFrozenSnapshot, usage BatchLineUsage) (decimal.Decimal, *billingexpr.Calculation, error) {
	if frozen == nil || frozen.QuotaPerUnit <= 0 || frozen.GroupRatio <= 0 || math.IsNaN(frozen.GroupRatio) || math.IsInf(frozen.GroupRatio, 0) {
		return decimal.Zero, nil, fmt.Errorf("batch pricing snapshot is invalid")
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CachedTokens < 0 || usage.CachedTokens > usage.InputTokens {
		return decimal.Zero, nil, fmt.Errorf("batch usage is invalid")
	}
	pricingTime := time.Unix(frozen.PricingTime, 0).UTC()
	request := billingexpr.RequestInput{PricingTime: &pricingTime, Body: frozen.LineParams[usage.CustomId], ExchangeRate: frozen.UsdExchangeRate}
	request.RecordCalculation = true
	output, trace, err := billingexpr.RunExprByHashWithRequest(frozen.Expr, frozen.ExprHash, batchTokenParams(frozen.Expr, usage), request)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("batch line pricing failed: %w", err)
	}
	if output < 0 || math.IsNaN(output) || math.IsInf(output, 0) {
		return decimal.Zero, nil, fmt.Errorf("batch line pricing produced an invalid amount")
	}
	value := decimal.NewFromFloat(output).Div(decimal.NewFromInt(1_000_000)).Mul(decimal.NewFromFloat(frozen.QuotaPerUnit)).Mul(decimal.NewFromFloat(frozen.GroupRatio))
	trace.Calculation.Add("input_tokens", "token", usage.InputTokens)
	trace.Calculation.Add("output_tokens", "token", usage.OutputTokens)
	trace.Calculation.Add("cache_tokens", "token", usage.CachedTokens)
	if billingexpr.UsedVars(frozen.Expr)["cr"] {
		trace.Calculation.Add("subtract", "token", batchTokenParams(frozen.Expr, usage).P, usage.InputTokens, usage.CachedTokens)
	}
	trace.Calculation.Add("batch_conversion", "quota", value.String(), output, 1000000, frozen.QuotaPerUnit, frozen.GroupRatio)
	return value, trace.Calculation, nil
}

// EstimateBatchLineInputTokens makes a pessimistic first-order input estimate
// from the line body size. It never underestimates to zero: pre-consume must
// hold a plausible budget even before any real token counts exist.
func EstimateBatchLineInputTokens(bodyBytes int64) int64 {
	estimated := bodyBytes
	if estimated < 16 {
		estimated = 16
	}
	return estimated
}

// estimateBatchJobQuota prices every line pessimistically (size-derived input
// estimate plus the line's output cap) under the frozen pricing time. Cache
// discounts are never assumed. The sum converts through the checked quota
// helpers so a pathological budget saturates instead of overflowing.
func estimateBatchJobQuota(frozen *model.BatchFrozenSnapshot, lines []BatchLineEstimate) (int, error) {
	if frozen == nil || len(lines) == 0 {
		return 0, fmt.Errorf("batch estimate requires lines")
	}
	total := decimal.Zero
	frozen.InitialCalculations = make(map[string]*billingexpr.Calculation, len(lines))
	for _, line := range lines {
		calculation := billingexpr.NewCalculation()
		output, err := batchBudgetOutput(frozen, line, calculation)
		if err != nil {
			return 0, err
		}
		value := decimal.NewFromFloat(output).Div(decimal.NewFromInt(1_000_000)).Mul(decimal.NewFromFloat(frozen.QuotaPerUnit)).Mul(decimal.NewFromFloat(frozen.GroupRatio))
		calculation.Add("batch_conversion", "quota", value.String(), output, 1000000, frozen.QuotaPerUnit, frozen.GroupRatio)
		if frozen.ContractFact != nil {
			before := value
			value, err = ApplyCustomerContractRatio(value, frozen.ContractFact)
			if err != nil {
				return 0, err
			}
			calculation.Add("contract_ratio", "quota", value.String(), before.String(), frozen.ContractFact.RatioString())
		}
		quota, clamp := common.QuotaFromDecimalChecked(value.Ceil())
		if clamp != nil {
			return 0, fmt.Errorf("batch line estimate exceeds the supported quota range")
		}
		calculation.Add("ceil", "quota", quota, value.String())
		frozen.InitialCalculations[line.CustomId] = calculation.Finish(quota)
		total = total.Add(decimal.NewFromInt(int64(quota)))
	}
	estimate, clamp := common.QuotaFromDecimalChecked(total)
	if clamp != nil {
		return 0, fmt.Errorf("batch estimate exceeds the supported quota range")
	}
	return estimate, nil
}

// batchPublicStatusForUpstream maps one upstream status to the platform's
// public batch status. Upstream completed stays finalizing until this
// platform has persisted the results.
func batchPublicStatusForUpstream(upstream string) string {
	switch upstream {
	case azurebatch.StatusValidating:
		return "validating"
	case azurebatch.StatusInProgress:
		return "in_progress"
	case azurebatch.StatusFinalizing, azurebatch.StatusCompleted:
		return "finalizing"
	case azurebatch.StatusCancelling:
		return "cancelling"
	case azurebatch.StatusCancelled:
		return "cancelled"
	case azurebatch.StatusFailed:
		return "failed"
	case azurebatch.StatusExpired:
		return "expired"
	default:
		return "in_progress"
	}
}

// ValidateBatchBillingConfiguration is also called by the admin option write.
func ValidateBatchBillingConfiguration(raw string) error {
	var expressions map[string]string
	if err := common.UnmarshalJsonStr(raw, &expressions); err != nil || expressions == nil {
		return fmt.Errorf("Batch prices must be a model-to-expression JSON object")
	}
	for name, expression := range expressions {
		if name == "" || expression == "" {
			return fmt.Errorf("Batch model and expression must be nonempty")
		}
		if _, err := billingexpr.CompileFromCache(expression); err != nil {
			return fmt.Errorf("invalid Batch expression for %s: %w", name, err)
		}
		budgetSnapshot := &model.BatchFrozenSnapshot{Expr: expression}
		if billingexpr.UsesExchangeRate(expression) {
			rate, rateErr := operation_setting.CurrentUsdExchangeRateContext()
			if rateErr != nil {
				return fmt.Errorf("Batch expression for %s requires a valid USDExchangeRate setting: %w", name, rateErr)
			}
			budgetSnapshot.UsdExchangeRate = rate
		}
		if _, err := batchBudgetOutput(budgetSnapshot, BatchLineEstimate{InputEst: 1000, OutputCap: 1000}); err != nil {
			return fmt.Errorf("Batch budget for %s: %w", name, err)
		}
	}
	return nil
}
