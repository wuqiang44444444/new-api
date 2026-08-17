package controller

import (
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
