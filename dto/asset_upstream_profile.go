package dto

import "fmt"

type AssetUpstreamProfile string

const (
	AssetUpstreamProfileNone       AssetUpstreamProfile = "none"
	AssetUpstreamProfileArk        AssetUpstreamProfile = "ark_assets"
	AssetUpstreamProfileRelay      AssetUpstreamProfile = "relay_assets"
	AssetUpstreamProfileJoyCreator AssetUpstreamProfile = "joycreator_assets"
	AssetUpstreamProfileOfficial   AssetUpstreamProfile = "official_action_assets"
)

func (p AssetUpstreamProfile) IsValid() bool {
	switch p {
	case "", AssetUpstreamProfileNone, AssetUpstreamProfileArk, AssetUpstreamProfileRelay, AssetUpstreamProfileJoyCreator, AssetUpstreamProfileOfficial:
		return true
	default:
		return false
	}
}

func (p AssetUpstreamProfile) IsRoutable() bool {
	return p == AssetUpstreamProfileArk || p == AssetUpstreamProfileRelay || p == AssetUpstreamProfileOfficial
}

func ValidateAssetUpstreamProfile(p AssetUpstreamProfile) error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported asset upstream profile %q", p)
	}
	return nil
}
