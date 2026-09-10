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
	hosttypes "github.com/QuantumNous/new-api/types"
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

// computeBatchLineModelQuota evaluates one line's model cost under the frozen
// pricing time and converts it to quota with checked saturation.
func computeBatchLineModelQuota(frozen *model.BatchFrozenSnapshot, usage BatchLineUsage) (int, *common.QuotaClamp, error) {
	value, err := batchLineModelCost(frozen, usage)
	if err != nil {
		return 0, nil, err
	}
	quota, clamp := common.QuotaFromDecimalChecked(value)
	return quota, clamp, nil
}

func batchLineModelCost(frozen *model.BatchFrozenSnapshot, usage BatchLineUsage) (decimal.Decimal, error) {
	if frozen == nil || frozen.QuotaPerUnit <= 0 || frozen.GroupRatio <= 0 || math.IsNaN(frozen.GroupRatio) || math.IsInf(frozen.GroupRatio, 0) {
		return decimal.Zero, fmt.Errorf("batch pricing snapshot is invalid")
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CachedTokens < 0 || usage.CachedTokens > usage.InputTokens {
		return decimal.Zero, fmt.Errorf("batch usage is invalid")
	}
	pricingTime := time.Unix(frozen.PricingTime, 0).UTC()
	request := billingexpr.RequestInput{PricingTime: &pricingTime, Body: frozen.LineParams[usage.CustomId]}
	output, _, err := billingexpr.RunExprByHashWithRequest(frozen.Expr, frozen.ExprHash, batchTokenParams(frozen.Expr, usage), request)
	if err != nil {
		return decimal.Zero, fmt.Errorf("batch line pricing failed: %w", err)
	}
	if output < 0 || math.IsNaN(output) || math.IsInf(output, 0) {
		return decimal.Zero, fmt.Errorf("batch line pricing produced an invalid amount")
	}
	return decimal.NewFromFloat(output).Div(decimal.NewFromInt(1_000_000)).Mul(decimal.NewFromFloat(frozen.QuotaPerUnit)).Mul(decimal.NewFromFloat(frozen.GroupRatio)), nil
}

// Apply all ratios before the single per-line integer conversion.
func computeBatchLineFinalQuota(frozen *model.BatchFrozenSnapshot, usage BatchLineUsage) (int, *common.QuotaClamp, error) {
	value, err := batchLineModelCost(frozen, usage)
	if err != nil {
		return 0, nil, err
	}
	if frozen.ContractFact != nil {
		value, err = ApplyCustomerContractRatio(value, frozen.ContractFact)
		if err != nil {
			return 0, nil, err
		}
	}
	quota, clamp := common.QuotaFromDecimalChecked(value)
	return quota, clamp, nil
}

// ApplyBatchContractRatio multiplies one line's model quota by the frozen
// contract discount.
func ApplyBatchContractRatio(modelQuota int, fact *hosttypes.ContractBillingFact) (decimal.Decimal, error) {
	value := decimal.NewFromInt(int64(modelQuota))
	if fact == nil {
		return value, nil
	}
	return ApplyCustomerContractRatio(value, fact)
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
	for _, line := range lines {
		output, err := batchBudgetOutput(frozen, line)
		if err != nil {
			return 0, err
		}
		value := decimal.NewFromFloat(output).Div(decimal.NewFromInt(1_000_000)).Mul(decimal.NewFromFloat(frozen.QuotaPerUnit)).Mul(decimal.NewFromFloat(frozen.GroupRatio))
		if frozen.ContractFact != nil {
			value, err = ApplyCustomerContractRatio(value, frozen.ContractFact)
			if err != nil {
				return 0, err
			}
		}
		quota, clamp := common.QuotaFromDecimalChecked(value.Ceil())
		if clamp != nil {
			return 0, fmt.Errorf("batch line estimate exceeds the supported quota range")
		}
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
		if _, err := batchBudgetOutput(&model.BatchFrozenSnapshot{Expr: expression}, BatchLineEstimate{InputEst: 1000, OutputCap: 1000}); err != nil {
			return fmt.Errorf("Batch budget for %s: %w", name, err)
		}
	}
	return nil
}
