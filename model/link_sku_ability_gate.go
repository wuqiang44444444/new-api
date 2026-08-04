package model

import (
	"fmt"
)

func ValidateLinkSKUAbilityBindings(channel *Channel) error {
	if channel == nil {
		return fmt.Errorf("Link ability channel is required")
	}
	settings := channel.GetOtherSettings()
	if settings.LinkImplementation.Empty() {
		return ValidateLinkImplementationRegistration(channel, &settings)
	}
	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	if err != nil {
		return err
	}
	for _, execution := range executions {
		if capability, registered := ResolveVideoSKUCapability(execution.LinkSKU); registered {
			if err := ValidateVideoSKUImplementation(capability, channel); err != nil {
				return err
			}
			continue
		}
		if _, registered := ResolveImageSKUCapability(execution.LinkSKU); registered {
			if err := ValidateChannelLinkImplementationForSKU(channel, execution.LinkSKU); err != nil {
				return err
			}
			continue
		}
		return fmt.Errorf("Link customer model %q resolves to unregistered SKU %q", execution.CustomerModel, execution.LinkSKU)
	}
	return nil
}

func validateLinkSKUAbilitiesByChannelID(channelID int) error {
	var channel Channel
	if err := DB.First(&channel, "id = ?", channelID).Error; err != nil {
		return err
	}
	return ValidateLinkSKUAbilityBindings(&channel)
}

func validateLinkSKUAbilitiesByTag(tag string) error {
	var channels []Channel
	if err := DB.Where("tag = ?", tag).Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		if err := ValidateLinkSKUAbilityBindings(&channels[i]); err != nil {
			return err
		}
	}
	return nil
}
