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
	VideoSKUSeedanceBytePlus:        "dc4761018ab640e588942cb69f42cac517b3027e9a4b7fd8d19c0a6e63f95987",
	VideoSKUSeedance20Oversea:       "80fbc021411a9280a5214acfae1a71f7643581517377c53c133a8e6ae0a57f24",
	VideoSKUDoubaoSeedance20260128:  "f2912b15555aff6472f636cb58755ae8552ba35e30f21bf330e6bc66cb2c364b",
	VideoSKUSeedance20Mini720P:      "e128cd92cd2f274b6459e38e1e95bd94c1a8e03d4b13fa48f4029cd02a4774f4",
	VideoSKUSeedance20SD2720P:       "aeb867058e92db199dda0e1bd90176c4a454ad68d50fd8efcd7cad96f0e84ab1",
	VideoSKUSeedance20Fast720P:      "9c7f199418b5c3f5bafd429f61085d4d289e59f7715f14e02c24c5e003126fc2",
	VideoSKUSeedance20Value720P:     "530ad1bf49b065bc4c2b10af817ca763d024b9e32c2e5b12883a603ef6f65ee0",
	VideoSKUSeedance20Standard720P:  "51617820c4201f587107d7a3531d66ef7c99dd2bd66138a076eabd5def5d2e90",
	VideoSKUSeedance20Value1080P:    "9a405d1dca87896cbc738aa4dcdb2cc908b1a28537f708e2afca064c64ef89c1",
	VideoSKUSeedance20Standard1080P: "315adf153be7a75650a4adefaacc9a082e22c8f8e8c4fa9b692811d6d4a446d0",
	VideoSKUSeedance20Value4K:       "85d057b051822aadead4885e3c20b51072f4cbd272ca232dcf30e1d1b61fc02e",
	VideoSKUSeedance20Standard4K:    "240e11f62b84b342452faf02dec240f79b48e884a045498c7828f6b4da9c7665",
	VideoSKUSeedance20ProPI720P:     "0cfb62ad039937f62e66f3ea095c2ff1a5d89b9d2c15f1052c02d4c92eb3e80b",
	VideoSKUSeedance20Standard:      "50056b167d80c6334b5aa8ad7adcf4b37f5c72c10e5fcefb52262ee053e75f4a",
	VideoSKUSeedance20Fast:          "91b0d2f7adca87d371a48f086463fadffefbaef4595bd2f25e15eac88b797392",
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
