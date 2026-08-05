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
	VideoSKUSeedance20Oversea:       "80fbc021411a9280a5214acfae1a71f7643581517377c53c133a8e6ae0a57f24",
	VideoSKUDoubaoSeedance20260128:  "f2912b15555aff6472f636cb58755ae8552ba35e30f21bf330e6bc66cb2c364b",
	VideoSKUSeedance20Mini720P:      "e128cd92cd2f274b6459e38e1e95bd94c1a8e03d4b13fa48f4029cd02a4774f4",
	VideoSKUSeedance20SD2720P:       "aeb867058e92db199dda0e1bd90176c4a454ad68d50fd8efcd7cad96f0e84ab1",
	VideoSKUSeedance20Fast720P:      "6d0a4adb863bf72cf24fccf6d386116114de4ccffec774e3e1027c56bf9500bb",
	VideoSKUSeedance20Value720P:     "93383a9c4d4573e92c77ceee4d907fc8be15406e0976ea40c243ca6da0a6a34b",
	VideoSKUSeedance20Standard720P:  "51617820c4201f587107d7a3531d66ef7c99dd2bd66138a076eabd5def5d2e90",
	VideoSKUSeedance20Value1080P:    "9a405d1dca87896cbc738aa4dcdb2cc908b1a28537f708e2afca064c64ef89c1",
	VideoSKUSeedance20Standard1080P: "315adf153be7a75650a4adefaacc9a082e22c8f8e8c4fa9b692811d6d4a446d0",
	VideoSKUSeedance20Value4K:       "85d057b051822aadead4885e3c20b51072f4cbd272ca232dcf30e1d1b61fc02e",
	VideoSKUSeedance20Standard4K:    "82a1429e820e600b0801bfa5e47f2dde81d2dc08638a4d14c3a934c6c6db9c13",
	VideoSKUSeedance20ProPI720P:     "0cfb62ad039937f62e66f3ea095c2ff1a5d89b9d2c15f1052c02d4c92eb3e80b",
	VideoSKUSeedance20Standard:      "8e9ff078808bec2194ff29964dedbc51e60d1c1a59d1d581781683871b774a04",
	VideoSKUSeedance20Fast:          "c11711ef1f1aa1f8768b2297a50c3e353b4640002ae3eee7e4ac7667a4f8285f",
	VideoSKUKlingV1:                 "2d5877f3e3257a2a89a92b172d333bb0851ad4200d459e9b012c09b903300c34",
	VideoSKUKlingV16:                "afdd30503fa4ded4fda82641e77802fdae2ea79329f2f373e951fe50264e5e3f",
	VideoSKUKlingV2Master:           "75e42fa0efda0c4b942c119fac32ea9205d978852ca95ec4e277028f6422bce5",
	VideoSKUJimengVGFMT2VL20:        "b1e1c293cd08dd6fdcc6419fd9b97392b0e784cbd4a212733c70516ed23d4e2e",
}

func ResolveVideoSKUImplementationCapability(publicModel string, ref dto.LinkImplementationRef) (VideoSKUCapability, bool) {
	publicModel = strings.TrimSpace(publicModel)
	implementation, ok := ResolveLinkImplementation(ref)
	if !ok || implementation.Deprecated || !slices.Contains(implementation.PublicSKUs, publicModel) {
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
