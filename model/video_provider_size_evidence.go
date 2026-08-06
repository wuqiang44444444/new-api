package model

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	FeicaiV2EvidenceVersion20260805   = "feicai-prod-2026-08-05-r1"
	FeicaiV2EvidenceVersion20260806   = "feicai-prod-2026-08-06-r2"
	FeicaiV2EvidenceVersion20260806R3 = "feicai-prod-2026-08-06-r3"
)

type VideoProviderSizeEvidence struct {
	ProviderSize    string  `json:"provider_size"`
	Multiplier      float64 `json:"multiplier"`
	BillingClass    string  `json:"billing_class"`
	EvidenceVersion string  `json:"evidence_version"`
}

type videoProviderSizeEvidenceKey struct {
	ImplementationID      string
	ImplementationVersion string
	ProviderModel         string
	Resolution            string
	Ratio                 string
}

var videoProviderSizeEvidenceRegistry = map[videoProviderSizeEvidenceKey]VideoProviderSizeEvidence{
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Mini720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "mini-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Standard720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Standard1080P, Resolution: ModelArkResolution1080P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-1080p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Fast720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "fast-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806,
	},
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Value720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "value-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806R3,
	},
	{
		ImplementationID: LinkImplementationFeicaiSeedanceVideos, ImplementationVersion: LinkImplementationVersionV2,
		ProviderModel: FeicaiProviderModelSeedance20Standard4K, Resolution: ModelArkResolution4K, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-4k-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806,
	},
}

func ResolveVideoProviderSizeEvidence(
	implementation dto.LinkImplementationRef,
	providerModel string,
	resolution string,
	ratio string,
) (VideoProviderSizeEvidence, bool) {
	key := videoProviderSizeEvidenceKey{
		ImplementationID:      strings.TrimSpace(implementation.ID),
		ImplementationVersion: strings.TrimSpace(implementation.Version),
		ProviderModel:         strings.TrimSpace(providerModel),
		Resolution:            strings.ToLower(strings.TrimSpace(resolution)),
		Ratio:                 strings.TrimSpace(ratio),
	}
	evidence, ok := videoProviderSizeEvidenceRegistry[key]
	if !ok || strings.TrimSpace(evidence.ProviderSize) == "" || evidence.Multiplier <= 0 ||
		math.IsNaN(evidence.Multiplier) || math.IsInf(evidence.Multiplier, 0) ||
		strings.TrimSpace(evidence.BillingClass) == "" || strings.TrimSpace(evidence.EvidenceVersion) == "" {
		return VideoProviderSizeEvidence{}, false
	}
	return evidence, true
}
