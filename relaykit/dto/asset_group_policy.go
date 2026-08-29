package dto

// GeneralAssetGroupPolicy defines how a Seedance asset protocol fulfills the
// unified northbound asset_group_id contract for general assets.
type GeneralAssetGroupPolicy string

const (
	GeneralAssetGroupPolicyNone            GeneralAssetGroupPolicy = "none"
	GeneralAssetGroupPolicyDefaultFallback GeneralAssetGroupPolicy = "default_fallback"
)

// GeneralAssetGroupPolicy returns the single code-backed policy used by both
// runtime routing and public model metadata.
func (p AssetUpstreamProtocol) GeneralAssetGroupPolicy() GeneralAssetGroupPolicy {
	switch p {
	case AssetUpstreamProtocolNone:
		return GeneralAssetGroupPolicyNone
	case AssetUpstreamProtocolVolcengineAction,
		AssetUpstreamProtocolBytePlusAction,
		AssetUpstreamProtocolArkAssetsV1,
		AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetUpstreamProtocolMoxingJoyCreatorV1,
		AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetUpstreamProtocolFunCloudMaterial,
		AssetUpstreamProtocolCMCCAICCV2:
		return GeneralAssetGroupPolicyDefaultFallback
	default:
		return ""
	}
}
