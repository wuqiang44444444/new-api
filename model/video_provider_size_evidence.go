package model

import (
	"math"
	"strings"
)

const (
	ModelArkResolution720P  = "720p"
	ModelArkResolution1080P = "1080p"
	ModelArkResolution4K    = "4k"

	FeicaiV2EvidenceVersion20260805   = "feicai-prod-2026-08-05-r1"
	FeicaiV2EvidenceVersion20260806   = "feicai-prod-2026-08-06-r2"
	FeicaiV2EvidenceVersion20260806R3 = "feicai-prod-2026-08-06-r3"
	FeicaiV3EvidenceVersion20260810   = "feicai-v3-prod-2026-08-10-r1"
)

type VideoProviderSizeEvidence struct {
	ProviderSize    string  `json:"provider_size"`
	Multiplier      float64 `json:"multiplier"`
	BillingClass    string  `json:"billing_class"`
	EvidenceVersion string  `json:"evidence_version"`
}

type videoProviderSizeEvidenceKey struct {
	ProviderModel string
	Resolution    string
	Ratio         string
}

var videoProviderSizeEvidenceRegistry = map[videoProviderSizeEvidenceKey]VideoProviderSizeEvidence{
	{
		ProviderModel: "seedance-2.0-vip-720p-mini-azhw-feicai", Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "mini-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ProviderModel: "seedance-2.0-vip-720p-azhw-feicai", Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ProviderModel: "seedance-2.0-vip-1080p-azhw-feicai", Resolution: ModelArkResolution1080P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-1080p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260805,
	},
	{
		ProviderModel: "seedance-2.0-vip-720p-fast-azhw-feicai", Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "fast-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806,
	},
	{
		ProviderModel: "seedance-2.0-933-720p-azhw-feicai", Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "value-720p-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806R3,
	},
	{
		ProviderModel: "seedance-2.0-vip-4k-azhw-feicai", Resolution: ModelArkResolution4K, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-4k-16-9", EvidenceVersion: FeicaiV2EvidenceVersion20260806,
	},
	{
		ProviderModel: FeicaiV3ProviderModelSeedance20Mini720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "mini-720p-16-9", EvidenceVersion: FeicaiV3EvidenceVersion20260810,
	},
	{
		ProviderModel: FeicaiV3ProviderModelSeedance20Fast720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "fast-720p-16-9", EvidenceVersion: FeicaiV3EvidenceVersion20260810,
	},
	{
		ProviderModel: FeicaiV3ProviderModelSeedance20Standard720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-720p-16-9", EvidenceVersion: FeicaiV3EvidenceVersion20260810,
	},
}

func ResolveVideoProviderSizeEvidence(
	providerModel string,
	resolution string,
	ratio string,
) (VideoProviderSizeEvidence, bool) {
	key := videoProviderSizeEvidenceKey{
		ProviderModel: strings.TrimSpace(providerModel),
		Resolution:    strings.ToLower(strings.TrimSpace(resolution)),
		Ratio:         strings.TrimSpace(ratio),
	}
	evidence, ok := videoProviderSizeEvidenceRegistry[key]
	if !ok || strings.TrimSpace(evidence.ProviderSize) == "" || evidence.Multiplier <= 0 ||
		math.IsNaN(evidence.Multiplier) || math.IsInf(evidence.Multiplier, 0) ||
		strings.TrimSpace(evidence.BillingClass) == "" || strings.TrimSpace(evidence.EvidenceVersion) == "" {
		return VideoProviderSizeEvidence{}, false
	}
	return evidence, true
}
