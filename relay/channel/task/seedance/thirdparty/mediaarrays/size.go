// Package mediaarrays implements the Seedance media-arrays protocol.
package mediaarrays

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

type VideoSize struct {
	Value           string
	Multiplier      float64
	BillingClass    string
	EvidenceVersion string
}

type videoSizeRegistryKey struct {
	ProviderModel string
	Resolution    string
	Ratio         string
}

// videoSizes is a package-local test overlay. Production evidence is owned by
// model so Ability routing, billing, and Provider conversion consume the
// same immutable registry.
var videoSizes = make(map[videoSizeRegistryKey]VideoSize)

func ResolveVideoSize(
	providerModel string,
	resolution string,
	ratio string,
) (VideoSize, bool) {
	key := videoSizeRegistryKey{
		ProviderModel: strings.TrimSpace(providerModel),
		Resolution:    strings.ToLower(strings.TrimSpace(resolution)),
		Ratio:         strings.TrimSpace(ratio),
	}
	size, ok := videoSizes[key]
	if ok && strings.TrimSpace(size.Value) != "" && size.Multiplier > 0 &&
		!math.IsNaN(size.Multiplier) && !math.IsInf(size.Multiplier, 0) &&
		strings.TrimSpace(size.BillingClass) != "" && strings.TrimSpace(size.EvidenceVersion) != "" {
		return size, true
	}
	evidence, ok := model.ResolveVideoProviderSizeEvidence(providerModel, resolution, ratio)
	if !ok {
		return VideoSize{}, false
	}
	size = VideoSize{
		Value: evidence.ProviderSize, Multiplier: evidence.Multiplier,
		BillingClass: evidence.BillingClass, EvidenceVersion: evidence.EvidenceVersion,
	}
	if strings.TrimSpace(size.Value) == "" || size.Multiplier <= 0 ||
		strings.TrimSpace(size.BillingClass) == "" || strings.TrimSpace(size.EvidenceVersion) == "" {
		return VideoSize{}, false
	}
	return size, true
}
