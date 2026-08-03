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
	VideoSKUSeedanceBytePlus:        "f9ddc5b4e9f16630c7d65641e27dd19adc0dd134c7eea30d9ffc43148becbcdd",
	VideoSKUSeedance20Oversea:       "bb716b78af9756be520e52640fbf374d162cbbe94788e0decd2a6518eb9e246b",
	VideoSKUDoubaoSeedance20260128:  "c77dfe5ae02678581b7773a1197caf7e081ed452be9fd7d724545cbb7a91cd4c",
	VideoSKUSeedance20Standard720P:  "4aa010d4f50138e0e876e512c2efa62c7fd324fa46298cddf37c5a2f6f6f715e",
	VideoSKUSeedance20Standard1080P: "3abd774e60d4c42fc5123ee82eba03cec17774c231ade846687aa0ee8e4dd064",
	VideoSKUSeedance20Value720P:     "69f2d9b03a96cfe0ea7936a30db6005ec2a0567c823a8b0a4add679ff4f46d1c",
	VideoSKUSeedance20Value1080P:    "fe99da31e182098cc8389a360582100d6f01fb29fc1b5199cf5ba8e8aec909c9",
	VideoSKUSeedance20Value4K:       "74d8897358274f96f8f6a74a73600b17b1236f6932b158201890bff480979d8b",
	VideoSKUSeedance20Standard:      "94c2cdc702e99aafed9ad74f90d49e8780ae5776aa3fdd091814ff49296ceaa4",
	VideoSKUSeedance20Fast:          "1c1298b9e94308b1d95a298dd3d02e85441479cc9fcd09bbaefd6bb4ee760794",
	VideoSKUKlingV1:                 "2d5877f3e3257a2a89a92b172d333bb0851ad4200d459e9b012c09b903300c34",
	VideoSKUKlingV16:                "afdd30503fa4ded4fda82641e77802fdae2ea79329f2f373e951fe50264e5e3f",
	VideoSKUKlingV2Master:           "75e42fa0efda0c4b942c119fac32ea9205d978852ca95ec4e277028f6422bce5",
	VideoSKUJimengVGFMT2VL20:        "b1e1c293cd08dd6fdcc6419fd9b97392b0e784cbd4a212733c70516ed23d4e2e",
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
