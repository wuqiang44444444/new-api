package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

// The adapter conformance hashes are pinned independently from the public
// registry. The implementation identity is resolved first; profile similarity
// alone can never select one of these declarations.
var videoSKUImplementationHashes = map[string]string{
	VideoSKUSeedanceBytePlus:       "f9ddc5b4e9f16630c7d65641e27dd19adc0dd134c7eea30d9ffc43148becbcdd",
	VideoSKUSeedance20Oversea:      "bb716b78af9756be520e52640fbf374d162cbbe94788e0decd2a6518eb9e246b",
	VideoSKUDoubaoSeedance20260128: "c77dfe5ae02678581b7773a1197caf7e081ed452be9fd7d724545cbb7a91cd4c",
	VideoSKUSeedance20Standard720P: "2c0bc18f5473343dc4f70ee8c3646e80fd1e12827ac0c6d088823b523df010ca",
	VideoSKUSeedance20Value720P:    "7f7505e2368c7f3312cc025fedf80a6dbe1d80aeb2220e92865b99b8c32d7916",
	VideoSKUSeedance20Standard:     "94c2cdc702e99aafed9ad74f90d49e8780ae5776aa3fdd091814ff49296ceaa4",
	VideoSKUSeedance20Fast:         "1c1298b9e94308b1d95a298dd3d02e85441479cc9fcd09bbaefd6bb4ee760794",
	VideoSKUKlingV1:                "2d5877f3e3257a2a89a92b172d333bb0851ad4200d459e9b012c09b903300c34",
	VideoSKUKlingV16:               "afdd30503fa4ded4fda82641e77802fdae2ea79329f2f373e951fe50264e5e3f",
	VideoSKUKlingV2Master:          "75e42fa0efda0c4b942c119fac32ea9205d978852ca95ec4e277028f6422bce5",
	VideoSKUJimengVGFMT2VL20:       "b1e1c293cd08dd6fdcc6419fd9b97392b0e784cbd4a212733c70516ed23d4e2e",
}

func ResolveVideoSKUImplementationCapability(publicModel string, ref dto.LinkImplementationRef) (VideoSKUCapability, bool) {
	publicModel = strings.TrimSpace(publicModel)
	implementation, ok := ResolveLinkImplementation(ref)
	if !ok || !slices.Contains(implementation.PublicSKUs, publicModel) {
		return VideoSKUCapability{}, false
	}
	capability, ok := ResolveVideoSKUCapability(publicModel)
	if !ok {
		return VideoSKUCapability{}, false
	}
	implementedHash, declared := videoSKUImplementationHashes[publicModel]
	if !declared {
		return VideoSKUCapability{}, false
	}
	capability.ContentHash = implementedHash
	return capability, true
}

func VideoSKUCapabilitiesEquivalent(public, implementation VideoSKUCapability) bool {
	return public.PublicModel != "" &&
		public.PublicModel == implementation.PublicModel &&
		public.ContractID == implementation.ContractID &&
		public.Version == implementation.Version &&
		public.ContentHash != "" &&
		public.ContentHash == videoSKUCapabilityHash(public) &&
		public.ContentHash == implementation.ContentHash &&
		implementation.ContentHash == videoSKUCapabilityHash(implementation)
}

func ValidateVideoSKUImplementation(public VideoSKUCapability, channel *Channel) error {
	if channel == nil {
		return fmt.Errorf("video SKU %q implementation channel is required", public.PublicModel)
	}
	settings := channel.GetOtherSettings()
	implementation, ok := ResolveVideoSKUImplementationCapability(public.PublicModel, settings.LinkImplementation)
	if !ok {
		return fmt.Errorf("video SKU %q has no implementation capability for channel Link implementation %q/%q", public.PublicModel, settings.LinkImplementation.ID, settings.LinkImplementation.Version)
	}
	if err := ValidateChannelLinkImplementationForSKU(channel, public.PublicModel); err != nil {
		return err
	}
	if !VideoSKUCapabilitiesEquivalent(public, implementation) {
		return fmt.Errorf("video SKU %q implementation capability is not equivalent to %s/%s", public.PublicModel, public.Version, public.ContentHash)
	}
	return nil
}

func normalizedVideoProfile(profile string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return VideoProfileOfficial
	}
	return profile
}
