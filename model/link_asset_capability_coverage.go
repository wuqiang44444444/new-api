package model

import (
	"fmt"
	"slices"
)

func ValidateLinkImplementationAssetCoverage(implementation LinkImplementation, linkSKU string) error {
	video, isVideo := ResolveVideoSKUCapability(linkSKU)
	if isVideo {
		if !video.SupportsLinkAssets {
			return nil
		}
		asset := implementation.AssetCapability
		if len(asset.ResolutionModes) == 0 {
			return fmt.Errorf("Link implementation %s/%s cannot resolve assets required by SKU %q", implementation.ID, implementation.Version, linkSKU)
		}
		if !linkAssetCountCovered(asset.MaxImages, video.MaxImages) || !linkAssetCountCovered(asset.MaxVideos, video.MaxVideos) || !linkAssetCountCovered(asset.MaxAudio, video.MaxAudio) {
			return fmt.Errorf("Link implementation %s/%s asset counts do not cover SKU %q", implementation.ID, implementation.Version, linkSKU)
		}
		for _, mediaType := range videoRequiredAssetMediaTypes(video) {
			if !slices.Contains(asset.MediaTypes, mediaType) {
				return fmt.Errorf("Link implementation %s/%s does not cover %s assets required by SKU %q", implementation.ID, implementation.Version, mediaType, linkSKU)
			}
		}
		return nil
	}
	if image, ok := ResolveImageSKUCapability(linkSKU); ok && image.SupportsLinkAssets && len(implementation.AssetCapability.ResolutionModes) == 0 {
		return fmt.Errorf("Link implementation %s/%s cannot resolve assets required by image SKU %q", implementation.ID, implementation.Version, linkSKU)
	}
	return nil
}

func linkAssetCountCovered(implementationMax, skuMax int) bool {
	return skuMax == 0 || implementationMax == 0 || implementationMax >= skuMax
}

func videoRequiredAssetMediaTypes(capability VideoSKUCapability) []string {
	mediaTypes := make([]string, 0, 3)
	if capability.MaxImages > 0 {
		mediaTypes = append(mediaTypes, "image")
	}
	if capability.MaxVideos > 0 {
		mediaTypes = append(mediaTypes, "video")
	}
	if capability.MaxAudio > 0 {
		mediaTypes = append(mediaTypes, "audio")
	}
	return mediaTypes
}
