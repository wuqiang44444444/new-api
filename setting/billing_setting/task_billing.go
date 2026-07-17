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

func ValidateBillingExpressionsJSON(value string, oldValue map[string]string) error {
	var expressions map[string]string
	if err := common.UnmarshalJsonStr(value, &expressions); err != nil {
		return err
	}
	for model, expression := range expressions {
		// 仅对新增或修改的表达式强制 tier()；未变更的存量表达式走 relaxed，避免阻塞历史配置保存。
		requireTier := oldValue[model] != expression
		if err := smokeTestExpr(expression, requireTier); err != nil {
			return fmt.Errorf("invalid billing expression for model %s: %w", model, err)
		}
	}
	return nil
}
