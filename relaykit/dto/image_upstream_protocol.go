package dto

import "fmt"

// ImageUpstreamProtocol identifies a code-backed southbound image adapter.
// Administrators select the protocol explicitly; model_mapping remains an
// independent customer-model to Provider-model mapping.
type ImageUpstreamProtocol string

const (
	ImageUpstreamProtocolFunCloudAIGCV2 ImageUpstreamProtocol = "funcloud_aigc_v2"
	ImageUpstreamProtocolMoxingImagesV1 ImageUpstreamProtocol = "moxing_images_v1"
)

func (p ImageUpstreamProtocol) IsValid() bool {
	switch p {
	case ImageUpstreamProtocolFunCloudAIGCV2, ImageUpstreamProtocolMoxingImagesV1:
		return true
	default:
		return false
	}
}

func ValidateImageUpstreamProtocol(p ImageUpstreamProtocol) error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported image upstream protocol %q", p)
	}
	return nil
}
