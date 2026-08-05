package mediaarrays

import "github.com/QuantumNous/new-api/model"

const feicaiV2Evidence20260805 = "feicai-prod-2026-08-05-r1"

func feicaiV2VerifiedVideoSizes() map[videoSizeRegistryKey]VideoSize {
	implementationID := model.LinkImplementationFeicaiSeedanceVideos
	implementationVersion := model.LinkImplementationVersionV2
	return map[videoSizeRegistryKey]VideoSize{
		{
			ImplementationID: implementationID, ImplementationVersion: implementationVersion,
			ProviderModel: model.FeicaiProviderModelSeedance20Mini720P,
			Resolution:    "720p", Ratio: "16:9",
		}: {
			Value: "1280x720", Multiplier: 1, BillingClass: "mini-720p-16-9", EvidenceVersion: feicaiV2Evidence20260805,
		},
		{
			ImplementationID: implementationID, ImplementationVersion: implementationVersion,
			ProviderModel: model.FeicaiProviderModelSeedance20Standard720P,
			Resolution:    "720p", Ratio: "16:9",
		}: {
			Value: "1280x720", Multiplier: 1, BillingClass: "standard-720p-16-9", EvidenceVersion: feicaiV2Evidence20260805,
		},
		{
			ImplementationID: implementationID, ImplementationVersion: implementationVersion,
			ProviderModel: model.FeicaiProviderModelSeedance20Standard1080P,
			Resolution:    "1080p", Ratio: "16:9",
		}: {
			Value: "1280x720", Multiplier: 1, BillingClass: "standard-1080p-16-9", EvidenceVersion: feicaiV2Evidence20260805,
		},
	}
}
