package billing_setting

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/config"
)

const TaskPreConsumeTokensOption = "task_billing_setting.preconsume_tokens"
const MaxTaskPreConsumeTokens = math.MaxInt32 / 2

type TaskBillingSetting struct {
	PreConsumeTokens map[string]int `json:"preconsume_tokens"`
}

var taskBillingSetting = TaskBillingSetting{
	PreConsumeTokens: make(map[string]int),
}

func init() {
	config.GlobalConfig.Register("task_billing_setting", &taskBillingSetting)
}

// GetTaskPreConsumeTokens returns the model-specific maximum billable-token
// estimate. Task pricing rejects missing values rather than using a magic
// process-wide fallback.
func GetTaskPreConsumeTokens(model string) (int, bool) {
	tokens, ok := taskBillingSetting.PreConsumeTokens[model]
	return tokens, ok && tokens > 0 && tokens <= MaxTaskPreConsumeTokens
}

func ValidateTaskPreConsumeTokensJSON(value string) error {
	var tokens map[string]int
	if err := common.UnmarshalJsonStr(value, &tokens); err != nil {
		return err
	}
	for model, upperBound := range tokens {
		if model == "" || upperBound <= 0 || upperBound > MaxTaskPreConsumeTokens {
			return fmt.Errorf(
				"task pre-consume token upper bound must be between 1 and %d for model %q",
				MaxTaskPreConsumeTokens,
				model,
			)
		}
	}
	return nil
}

// ValidateOneBillingExpression validates a single model expression with the
// generic token contract: new or modified expressions must wrap prices in tier().
// Unchanged generic expressions retain the existing tier requirement exemption.
// Seedance and native plugin expressions are validated by their own owners.
func ValidateOneBillingExpression(modelName, expression, oldValue string) error {
	requireTier := oldValue != expression
	if err := smokeTestGenericExpression(expression, requireTier); err != nil {
		return fmt.Errorf("invalid billing expression for model %s: %w", modelName, err)
	}
	return nil
}

// smokeTestGenericExpression validates finite, non-negative token prices with
// generic request fields and requires tier() when requested. It duplicates the
// shared smoke vectors loop locally so setting/billing_setting/tiered_billing.go
// stays byte-identical to upstream (allowed narrow duplication per the
// minimal-invasion rule).
func smokeTestGenericExpression(exprStr string, requireTier bool) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	// This generic model has no declared usage facts, including for nil branches.
	if billingexpr.UsedVars(exprStr)["u"] {
		return fmt.Errorf("expression references u() but the model has no task plugin usage schema")
	}

	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}

	requests := billingExprSmokeRequests()

	for _, v := range vectors {
		for _, request := range requests {
			result, trace, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if math.IsNaN(result) || math.IsInf(result, 0) || result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result must be finite and non-negative, got %f", v.P, v.C, result)
			}
			if requireTier && trace.MatchedTier == "" {
				return fmt.Errorf("billing expression must wrap every price branch with tier(name, value)")
			}
		}
	}
	return nil
}
