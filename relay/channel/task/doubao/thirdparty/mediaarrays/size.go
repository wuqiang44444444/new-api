package mediaarrays

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

type VideoSize struct {
	Value           string
	Multiplier      float64
	BillingClass    string
	EvidenceVersion string
}

type videoSizeRegistryKey struct {
	ImplementationID      string
	ImplementationVersion string
	ProviderModel         string
	Resolution            string
	Ratio                 string
}

// videoSizes contains only size combinations proven with the exact frozen
// implementation, provider model and production credential. Research examples
// and values inferred from a sibling model deliberately do not belong here.
var videoSizes = feicaiV2VerifiedVideoSizes()

func ResolveVideoSize(
	implementation dto.LinkImplementationRef,
	providerModel string,
	resolution string,
	ratio string,
) (VideoSize, bool) {
	key := videoSizeRegistryKey{
		ImplementationID:      strings.TrimSpace(implementation.ID),
		ImplementationVersion: strings.TrimSpace(implementation.Version),
		ProviderModel:         strings.TrimSpace(providerModel),
		Resolution:            strings.ToLower(strings.TrimSpace(resolution)),
		Ratio:                 strings.TrimSpace(ratio),
	}
	size, ok := videoSizes[key]
	if !ok || strings.TrimSpace(size.Value) == "" || size.Multiplier <= 0 ||
		strings.TrimSpace(size.BillingClass) == "" || strings.TrimSpace(size.EvidenceVersion) == "" {
		return VideoSize{}, false
	}
	return size, true
}
