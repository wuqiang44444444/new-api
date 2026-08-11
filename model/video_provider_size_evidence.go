package model

import (
	"math"
	"strings"
)

const (
	ModelArkResolution720P  = "720p"
	ModelArkResolution1080P = "1080p"
	ModelArkResolution4K    = "4k"

	FeicaiEvidenceVersion20260810 = "feicai-prod-2026-08-10-r1"
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
		ProviderModel: FeicaiProviderModelSeedance20Mini720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "mini-720p-16-9", EvidenceVersion: FeicaiEvidenceVersion20260810,
	},
	{
		ProviderModel: FeicaiProviderModelSeedance20Fast720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "fast-720p-16-9", EvidenceVersion: FeicaiEvidenceVersion20260810,
	},
	{
		ProviderModel: FeicaiProviderModelSeedance20Standard720P, Resolution: ModelArkResolution720P, Ratio: "16:9",
	}: {
		ProviderSize: "1280x720", Multiplier: 1, BillingClass: "standard-720p-16-9", EvidenceVersion: FeicaiEvidenceVersion20260810,
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
