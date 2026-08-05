package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const (
	ModelArkResolution480P  = "480p"
	ModelArkResolution720P  = "720p"
	ModelArkResolution1080P = "1080p"
	ModelArkResolution4K    = "4k"
)

var modelArkCanonicalResolutions = []string{
	ModelArkResolution480P,
	ModelArkResolution720P,
	ModelArkResolution1080P,
	ModelArkResolution4K,
}

var modelArkCanonicalRatios = []string{
	"16:9",
	"4:3",
	"1:1",
	"3:4",
	"9:16",
	"21:9",
	"adaptive",
}

func canonicalModelArkRatios() []string {
	return append([]string(nil), modelArkCanonicalRatios...)
}

func validateModelArkCapabilityVocabulary(capability VideoSKUCapability) error {
	if capability.ContractID != string(dto.VideoContractModelArkV3) {
		return nil
	}
	resolutions := append([]string(nil), capability.Resolutions...)
	if resolution := strings.TrimSpace(capability.Resolution); resolution != "" {
		resolutions = append(resolutions, resolution)
	}
	if resolution := strings.TrimSpace(capability.DefaultResolution); resolution != "" {
		resolutions = append(resolutions, resolution)
	}
	for _, resolution := range resolutions {
		if !slices.Contains(modelArkCanonicalResolutions, resolution) {
			return fmt.Errorf("video SKU %q contains non-canonical resolution %q", capability.PublicModel, resolution)
		}
	}
	for _, ratio := range append(append([]string(nil), capability.Ratios...), capability.DefaultRatio) {
		if ratio != "" && !slices.Contains(modelArkCanonicalRatios, ratio) {
			return fmt.Errorf("video SKU %q contains non-canonical ratio %q", capability.PublicModel, ratio)
		}
	}
	for _, combination := range capability.ResolutionRatioCombinations {
		if !slices.Contains(modelArkCanonicalResolutions, combination.Resolution) ||
			!slices.Contains(modelArkCanonicalRatios, combination.Ratio) {
			return fmt.Errorf("video SKU %q contains non-canonical resolution/ratio combination", capability.PublicModel)
		}
	}
	return nil
}
