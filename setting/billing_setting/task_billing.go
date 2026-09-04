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

func ValidateBillingExpressionsJSON(
	value string,
	oldValue map[string]string,
	taskProbeExtraFieldsByModel map[string]map[string]any,
) error {
	var expressions map[string]string
	if err := common.UnmarshalJsonStr(value, &expressions); err != nil {
		return err
	}
	for model, expression := range expressions {
		extraFields, taskModel := taskProbeExtraFieldsByModel[model]
		if err := ValidateOneBillingExpression(model, expression, oldValue[model], extraFields, taskModel); err != nil {
			return err
		}
	}
	return nil
}

// ValidateOneBillingExpression validates a single model expression with the
// Link contract semantics: only new or modified expressions must wrap prices
// in tier(), unchanged legacy expressions stay relaxed so saving historical
// configuration never blocks.
func ValidateOneBillingExpression(modelName, expression, oldValue string, extraFields map[string]any, taskModel bool) error {
	requireTier := oldValue != expression
	if err := smokeTestLinkExpr(expression, requireTier, taskModel, extraFields); err != nil {
		return fmt.Errorf("invalid billing expression for model %s: %w", modelName, err)
	}
	return nil
}

// smokeTestLinkExpr validates a Link contract expression: compilable, vector
// results non-negative, tier() wrapping required when requireTier is set, and
// task models probed with protocol-specific request fields. It duplicates the
// shared smoke vectors loop locally so setting/billing_setting/tiered_billing.go
// stays byte-identical to upstream (allowed narrow duplication per the
// minimal-invasion rule).
func smokeTestLinkExpr(exprStr string, requireTier bool, taskModel bool, taskProbeExtraFields map[string]any) error {
	if _, err := billingexpr.CompileFromCache(exprStr); err != nil {
		return err
	}
	if !taskModel {
		usageKeys := billingexpr.UsedUsageKeys(exprStr)
		if len(usageKeys) > 0 {
			return fmt.Errorf("expression references usage keys %v but the model has no task plugin usage schema", usageKeys)
		}
	}

	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}

	requests := billingExprSmokeRequests()
	if taskModel {
		var err error
		requests, err = taskBillingSmokeRequests(taskProbeExtraFields)
		if err != nil {
			return err
		}
	}

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
