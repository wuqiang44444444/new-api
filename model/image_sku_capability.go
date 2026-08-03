package model

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const ImageSKUCapabilityVersionV1 = "public-image-contract-v1"

type ImageSKUCapability struct {
	PublicModel        string   `json:"public_model"`
	ContractID         string   `json:"contract_id"`
	Version            string   `json:"version"`
	ContentHash        string   `json:"content_hash"`
	RequestFields      []string `json:"request_fields"`
	RequiredFields     []string `json:"required_fields"`
	Sizes              []string `json:"sizes"`
	AspectRatios       []string `json:"aspect_ratios,omitempty"`
	MaxPromptRunes     int      `json:"max_prompt_runes"`
	MaxInputImages     int      `json:"max_input_images"`
	MaxOutputImages    uint     `json:"max_output_images"`
	SupportsStream     bool     `json:"supports_stream"`
	SupportsLinkAssets bool     `json:"supports_link_assets"`
	TaskContract       string   `json:"task_contract"`
}

var imageSKUCapabilities = buildImageSKUCapabilities()

func buildImageSKUCapabilities() map[string]ImageSKUCapability {
	commonFields := []string{"model", "prompt", "image", "size", "n", "response_format", "stream"}
	capabilities := []ImageSKUCapability{
		{
			PublicModel: "seedream-5-moxing", RequestFields: append(append([]string(nil), commonFields...), "watermark"),
			Sizes: []string{"2K", "3K"}, MaxInputImages: 14,
		},
		{
			PublicModel: "seedream-5-qihang", RequestFields: append([]string(nil), commonFields...),
			Sizes: []string{"2K"}, MaxInputImages: 2,
		},
		{
			PublicModel: "nano-banana-2", RequestFields: append(append([]string(nil), commonFields...), "aspect_ratio"),
			Sizes: []string{"1K", "2K", "4K"}, MaxInputImages: 10,
			AspectRatios: []string{"1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "21:9"},
		},
	}
	result := make(map[string]ImageSKUCapability, len(capabilities))
	for _, capability := range capabilities {
		capability.ContractID = "newapi.images.generations.v1"
		capability.Version = ImageSKUCapabilityVersionV1
		capability.RequiredFields = []string{"model", "prompt", "size"}
		capability.MaxPromptRunes = 3000
		capability.MaxOutputImages = 1
		capability.SupportsStream = false
		// The common image Link Resolver is not published yet. Provider URL
		// support alone is not sufficient to turn this customer capability on.
		capability.SupportsLinkAssets = false
		capability.TaskContract = "shared_image_task"
		capability.ContentHash = imageSKUCapabilityHash(capability)
		result[capability.PublicModel] = capability
	}
	return result
}

func imageSKUCapabilityHash(capability ImageSKUCapability) string {
	capability.ContentHash = ""
	payload, err := common.Marshal(capability)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func ResolveImageSKUCapability(publicModel string) (ImageSKUCapability, bool) {
	capability, ok := imageSKUCapabilities[strings.TrimSpace(publicModel)]
	if !ok {
		return ImageSKUCapability{}, false
	}
	capability.RequestFields = append([]string(nil), capability.RequestFields...)
	capability.RequiredFields = append([]string(nil), capability.RequiredFields...)
	capability.Sizes = append([]string(nil), capability.Sizes...)
	capability.AspectRatios = append([]string(nil), capability.AspectRatios...)
	return capability, true
}

func IsRegisteredLinkSKU(publicModel string) bool {
	if _, ok := ResolveVideoSKUCapability(publicModel); ok {
		return true
	}
	_, ok := ResolveImageSKUCapability(publicModel)
	return ok
}
