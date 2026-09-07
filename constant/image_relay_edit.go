package constant

import "github.com/QuantumNous/new-api/relaykit/dto"

// ImageRelayInputLimits reads the same model profiles used for generation and discovery.
func ImageRelayInputLimits(protocol dto.ImageUpstreamProtocol, model string) (count, bytes int, ok bool) {
	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		for _, profile := range funCloudImageProfiles {
			if profile.providerModel == model {
				return profile.maxInputs, profile.maxInputBytes, true
			}
		}
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		for _, profile := range moxingImageProfiles {
			if profile.providerModel == model {
				return profile.maxInputs, profile.maxInputBytes, true
			}
		}
	}
	return 0, 0, false
}

// ImageRelayRequiresInputPricing identifies profiles whose published multi-input
// contract requires an input-count expression. It does not set a customer price.
func ImageRelayRequiresInputPricing(protocol dto.ImageUpstreamProtocol, model string) bool {
	switch protocol {
	case dto.ImageUpstreamProtocolFunCloudAIGCV2:
		for _, profile := range funCloudImageProfiles {
			if profile.providerModel == model {
				return profile.requiresInputPricing
			}
		}
	case dto.ImageUpstreamProtocolMoxingImagesV1:
		for _, profile := range moxingImageProfiles {
			if profile.providerModel == model {
				return profile.requiresInputPricing
			}
		}
	}
	return false
}
