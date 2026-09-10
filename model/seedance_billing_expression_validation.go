package model

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"gorm.io/gorm"
)

// ValidateSeedanceBillingExpression gives an explicitly configured Seedance
// model its Link probe before native plugin model/alias lookup can claim it.
// Other models retain the native validation path.
func ValidateSeedanceBillingExpression(modelName, expression string) (bool, error) {
	return validateSeedanceBillingExpression(DB, modelName, expression)
}

func validateSeedanceBillingExpression(db *gorm.DB, modelName, expression string) (bool, error) {
	if db == nil {
		return false, nil
	}
	channels, err := getSeedanceChannelsForBillingValidation(db, modelName)
	if err != nil {
		return true, err
	}
	for i := range channels {
		extraFields := dto.SeedanceBillingProbeValidationExtraFields(channels[i].GetOtherSettings().VideoUpstreamProtocol)
		if err := billing_setting.ValidateOneBillingExpression(modelName, expression, billing_setting.GetBillingExprCopy()[modelName], extraFields, true); err != nil {
			return true, err
		}
	}
	return len(channels) > 0, nil
}
