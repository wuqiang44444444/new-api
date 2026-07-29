package billing_statement_setting

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const ContextThresholdsOption = "billing_statement_setting.context_thresholds"
const MaxContextThresholdTokens = math.MaxInt32

type BillingStatementSetting struct {
	ContextThresholds map[string]int `json:"context_thresholds"`
}

var billingStatementSetting = BillingStatementSetting{
	ContextThresholds: make(map[string]int),
}

func init() {
	config.GlobalConfig.Register("billing_statement_setting", &billingStatementSetting)
}

func GetContextThreshold(model string) (int64, bool) {
	threshold, ok := billingStatementSetting.ContextThresholds[model]
	return int64(threshold), ok && threshold > 0 && threshold <= MaxContextThresholdTokens
}

func ValidateContextThresholdsJSON(value string) error {
	var thresholds map[string]int
	if err := common.UnmarshalJsonStr(value, &thresholds); err != nil {
		return err
	}
	if thresholds == nil {
		return fmt.Errorf("context thresholds must be a JSON object")
	}
	for model, threshold := range thresholds {
		if model == "" || model != strings.TrimSpace(model) {
			return fmt.Errorf("context threshold model name must be non-empty and cannot contain surrounding whitespace")
		}
		if threshold <= 0 || threshold > MaxContextThresholdTokens {
			return fmt.Errorf(
				"context threshold must be between 1 and %d for model %q",
				MaxContextThresholdTokens,
				model,
			)
		}
	}
	return nil
}
