package billing_setting

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
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
	if err := smokeTestExpr(expression, requireTier, taskModel, extraFields); err != nil {
		return fmt.Errorf("invalid billing expression for model %s: %w", modelName, err)
	}
	return nil
}
