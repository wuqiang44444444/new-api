package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// validateSeedanceBillingExpression gives an explicitly configured Seedance
// model its Link probe before native plugin model/alias lookup can claim it.
// Other models retain the native validation path.
func validateSeedanceBillingExpression(modelName, expression string) (bool, error) {
	if model.DB == nil {
		return false, nil
	}
	channels, err := model.GetEnabledSeedanceChannelsForBillingValidation()
	if err != nil {
		return true, err
	}
	for i := range channels {
		for _, customerModel := range channels[i].GetModels() {
			if strings.TrimSpace(customerModel) != modelName {
				continue
			}
			extraFields := seedance.BillingProbeValidationExtraFields(channels[i].GetOtherSettings().VideoUpstreamProtocol)
			return true, billing_setting.ValidateOneBillingExpression(modelName, expression, billing_setting.GetBillingExprCopy()[modelName], extraFields, true)
		}
	}
	return false, nil
}
