package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/pkg/seedancebilling"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"gorm.io/gorm"
)

// ValidateSeedanceBillingExpression gives an explicitly configured Seedance
// model its Link contract before native plugin model/alias lookup can claim it.
// Other models retain the native validation path.
func ValidateSeedanceBillingExpression(modelName, expression string) (bool, error) {
	return validateSeedanceBillingExpression(DB, modelName, expression)
}

// All save entries validate the same Seedance USD contract, including unchanged
// expressions. Historical task snapshots are not current pricing configuration.
func validateSeedanceBillingExpression(db *gorm.DB, modelName, expression string) (bool, error) {
	if db == nil {
		return false, nil
	}
	channels, err := getSeedanceChannelsForBillingValidation(db, modelName)
	if err != nil {
		return true, err
	}
	if len(channels) == 0 {
		return false, nil
	}
	for i := range channels {
		schema := StandardVideoBillingFields(channels[i].Type, channels[i].GetOtherSettings().VideoUpstreamProtocol)
		if err := seedancebilling.ValidateTaskExpression(expression, schema); err != nil {
			return true, fmt.Errorf("model %s: %w", modelName, err)
		}
	}
	return true, nil
}

// validateSeedancePricingConfiguration checks the final replacement configuration,
// including removed keys. Numeric bounds are owned by billing_setting.
func validateSeedancePricingConfiguration(db *gorm.DB, name string, values PricingValues) error {
	channels, err := getSeedanceChannelsForBillingValidation(db, name)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return nil
	}
	if values["billing_setting.billing_mode"] != "tiered_expr" {
		return fmt.Errorf("model %s requires tiered_expr billing", name)
	}
	expression, _ := values["billing_setting.billing_expr"].(string)
	if expression == "" {
		return fmt.Errorf("model %s requires a customer-key billing expression", name)
	}
	if _, exists := values[billing_setting.TaskPreConsumeTokensOption]; exists {
		return nil
	}
	for _, channel := range channels {
		if standardVideoRequiresTokenBudget(channel, expression) {
			return fmt.Errorf("model %s task pre-consume token upper bound is required", name)
		}
	}
	return nil
}
