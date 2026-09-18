// Package geminiimage defines the size contract shared by Gemini image relay
// and public metadata. It does not grant model routing eligibility.
package geminiimage

import (
	"fmt"
	"slices"
	"strings"
)

const twoKLongEdge = 1700
const fourKLongEdge = 3000

type SizePolicy struct {
	MaxDimension int
	NativeOutput bool
	Sizes        []string
	AspectRatios []string
}

func Policy(model string) SizePolicy {
	maxDimension := 4096
	if model == "gemini-3.1-flash-lite-image" {
		return SizePolicy{MaxDimension: 1024, NativeOutput: true, Sizes: []string{"auto", "1024x1024"}, AspectRatios: []string{"1:1"}}
	}
	return SizePolicy{MaxDimension: maxDimension, AspectRatios: []string{
		"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9",
	}}
}

// Resolve preserves auto by returning empty configuration. Explicit dimensions
// must satisfy both the family ratio contract and the model resolution limit.
func Resolve(model, size string) (aspectRatio, imageSize string, err error) {
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		return "", "", nil
	}
	policy := Policy(model)
	if policy.NativeOutput {
		if slices.Contains(policy.Sizes, size) {
			return policy.AspectRatios[0], "1K", nil
		}
		return "", "", fmt.Errorf("size must be auto or 1024x1024 for native 1K output")
	}
	w, h, parseErr := ParsePixels(size)
	if parseErr == nil && w <= policy.MaxDimension && h <= policy.MaxDimension {
		for _, ratio := range policy.AspectRatios {
			var rw, rh int
			_, _ = fmt.Sscanf(ratio, "%d:%d", &rw, &rh)
			if w*rh == h*rw {
				return ratio, Resolution(w, h), nil
			}
		}
	}
	return "", "", fmt.Errorf("size must be auto or a supported WxH size with an exact aspect ratio (dimensions up to %d)", policy.MaxDimension)
}

// ParsePixels bounds accumulation before integer multiplication can overflow.
func ParsePixels(size string) (int, int, error) {
	x := strings.Index(strings.ToLower(size), "x")
	if x <= 0 || x == len(size)-1 {
		return 0, 0, fmt.Errorf("invalid size %q", size)
	}
	var dimensions [2]int
	for i, token := range []string{size[:x], size[x+1:]} {
		for _, r := range token {
			if r < '0' || r > '9' {
				return 0, 0, fmt.Errorf("invalid size %q", size)
			}
			dimensions[i] = dimensions[i]*10 + int(r-'0')
			if dimensions[i] > 1_000_000 {
				return 0, 0, fmt.Errorf("size dimension is out of range")
			}
		}
		if dimensions[i] == 0 {
			return 0, 0, fmt.Errorf("size dimension must be positive")
		}
	}
	return dimensions[0], dimensions[1], nil
}

func Resolution(width, height int) string {
	longEdge := max(width, height)
	switch {
	case longEdge >= fourKLongEdge:
		return "4K"
	case longEdge >= twoKLongEdge:
		return "2K"
	default:
		return "1K"
	}
}
