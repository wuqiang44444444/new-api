package model

import (
	"fmt"
	"strings"
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

// ValidateLinkSKUAbilityPublicationReadiness applies release-only evidence
// gates after the structural implementation/binding checks. Disabled channels
// may be configured before evidence is complete, but enabling their Channel or
// Ability must not create a publication that no request can satisfy.
func ValidateLinkSKUAbilityPublicationReadiness(channel *Channel) error {
	if err := ValidateLinkSKUAbilityBindings(channel); err != nil {
		return err
	}
	settings := channel.GetOtherSettings()
	if normalizedVideoProfile(string(settings.VideoUpstreamProfile)) != VideoProfileJSONMediaArrays {
		return nil
	}
	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	if err != nil {
		return err
	}
	for _, execution := range executions {
		capability, registered := ResolveVideoSKUCapability(execution.LinkSKU)
		if !registered {
			return fmt.Errorf("Link customer model %q resolves to unregistered SKU %q", execution.CustomerModel, execution.LinkSKU)
		}
		combinations := capability.ResolutionRatioCombinations
		if len(combinations) == 0 {
			resolutions := capability.Resolutions
			if strings.TrimSpace(capability.Resolution) != "" {
				resolutions = []string{capability.Resolution}
			}
			for _, resolution := range resolutions {
				for _, ratio := range capability.Ratios {
					combinations = append(combinations, VideoResolutionRatioCombination{Resolution: resolution, Ratio: ratio})
				}
			}
		}
		if len(combinations) == 0 {
			return fmt.Errorf(
				"video SKU %q has no verified provider ratio/size evidence and cannot be published",
				strings.TrimSpace(execution.LinkSKU),
			)
		}
		for _, combination := range combinations {
			if _, verified := ResolveVideoProviderSizeEvidence(
				settings.LinkImplementation,
				execution.ProviderModel,
				combination.Resolution,
				combination.Ratio,
			); !verified {
				return fmt.Errorf(
					"video SKU %q has no verified provider ratio/size evidence for implementation %q/%q, Provider model %q, resolution %q, ratio %q and cannot be published",
					strings.TrimSpace(execution.LinkSKU),
					strings.TrimSpace(settings.LinkImplementation.ID),
					strings.TrimSpace(settings.LinkImplementation.Version),
					strings.TrimSpace(execution.ProviderModel),
					strings.TrimSpace(combination.Resolution),
					strings.TrimSpace(combination.Ratio),
				)
			}
		}
	}
	return nil
}
