package controller

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// validateBillingExpressionsOption resolves protocol-only probe fields from
// the selected Seedance Channel. Customer model names are treated only as
// exact configured keys and never as protocol hints.
func validateBillingExpressionsOption(value string) error {
	channels, err := model.GetEnabledSeedanceChannelsForBillingValidation()
	if err != nil {
		return err
	}
	extraFieldsByModel := make(map[string]map[string]any)
	for i := range channels {
		extraFields := seedance.BillingProbeValidationExtraFields(
			channels[i].GetOtherSettings().VideoUpstreamProtocol,
		)
		for _, customerModel := range channels[i].GetModels() {
			customerModel = strings.TrimSpace(customerModel)
			if customerModel == "" {
				continue
			}
			extraFieldsByModel[customerModel] = extraFields
		}
	}

	return billing_setting.ValidateBillingExpressionsJSON(
		value,
		billing_setting.GetBillingExprCopy(),
		extraFieldsByModel,
	)
}

// validateLinkBillingExpression validates one non-plugin model expression with
// Link contract semantics: changed expressions must wrap prices in tier(), and
// Link task models are probed with protocol-specific extra request fields.
func validateLinkBillingExpression(modelName string, expression string) error {
	oldValues := billing_setting.GetBillingExprCopy()
	if model.DB == nil {
		// Unit-test context without a database: skip Link task probing.
		return billing_setting.ValidateOneBillingExpression(modelName, expression, oldValues[modelName], nil, false)
	}
	channels, err := model.GetEnabledSeedanceChannelsForBillingValidation()
	if err != nil {
		// Channel lookup is only the probe-field source; on failure degrade to
		// base validation so compile and usage-key gates still reject saves.
		return billing_setting.ValidateOneBillingExpression(modelName, expression, oldValues[modelName], nil, false)
	}
	for i := range channels {
		if !slices.Contains(channels[i].GetModels(), modelName) {
			continue
		}
		extraFields := seedance.BillingProbeValidationExtraFields(
			channels[i].GetOtherSettings().VideoUpstreamProtocol,
		)
		return billing_setting.ValidateOneBillingExpression(modelName, expression, oldValues[modelName], extraFields, true)
	}
	return billing_setting.ValidateOneBillingExpression(modelName, expression, oldValues[modelName], nil, false)
}
