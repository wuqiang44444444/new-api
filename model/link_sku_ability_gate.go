package model

import (
	"fmt"
	"strings"
)

func ValidateLinkSKUAbilityBindings(channel *Channel) error {
	if channel == nil {
		return fmt.Errorf("Link ability channel is required")
	}
	for _, rawModel := range strings.Split(channel.Models, ",") {
		publicSKU := strings.TrimSpace(rawModel)
		if capability, registered := ResolveVideoSKUCapability(publicSKU); registered {
			if err := ValidateVideoSKUImplementation(capability, channel); err != nil {
				return err
			}
			continue
		}
		if _, registered := ResolveImageSKUCapability(publicSKU); registered {
			if err := ValidateChannelLinkImplementationForSKU(channel, publicSKU); err != nil {
				return err
			}
		}
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
