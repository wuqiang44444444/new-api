package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func validateMoxingImageChannelSettings(channel *Channel, settings *dto.ChannelSettings, otherSettings *dto.ChannelOtherSettings) error {
	if channel == nil || channel.Type != constant.ChannelTypeMoxingImage {
		return nil
	}
	if settings != nil && settings.PassThroughBodyEnabled {
		return fmt.Errorf("Moxing image channels do not allow request body pass-through")
	}
	if channel.ParamOverride != nil && strings.TrimSpace(*channel.ParamOverride) != "" {
		return fmt.Errorf("Moxing image channels do not allow parameter overrides")
	}
	if otherSettings != nil && otherSettings.AdvancedCustom != nil {
		return fmt.Errorf("Moxing image channels do not allow advanced custom routes")
	}

	models := channel.GetModels()
	if len(models) == 0 {
		return fmt.Errorf("Moxing image channels require at least one customer model")
	}
	customerModels := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		customerModel := strings.TrimSpace(modelName)
		if customerModel == "" {
			return fmt.Errorf("Moxing image channels require non-empty customer model names")
		}
		if customerModel != modelName {
			return fmt.Errorf("Moxing image customer model %q must not contain surrounding whitespace", modelName)
		}
		if _, duplicate := customerModels[customerModel]; duplicate {
			return fmt.Errorf("Moxing image customer model %q is duplicated", customerModel)
		}
		customerModels[customerModel] = struct{}{}
	}

	mapping := make(map[string]string)
	modelMapping := strings.TrimSpace(channel.GetModelMapping())
	if modelMapping != "" && modelMapping != "{}" {
		if err := common.UnmarshalJsonStr(modelMapping, &mapping); err != nil {
			return fmt.Errorf("Moxing image channel model_mapping must be a JSON object")
		}
	}
	for customerModel := range customerModels {
		providerModel, _, err := ResolveModelMapping(customerModel, mapping)
		if err != nil {
			return fmt.Errorf("model_mapping for customer model %q is invalid: %w", customerModel, err)
		}
		if _, supported := constant.MoxingImageFixedSize(providerModel); !supported {
			return fmt.Errorf("customer model %q resolves to unsupported Moxing image Provider model %q", customerModel, providerModel)
		}
	}
	return nil
}
