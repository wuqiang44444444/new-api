package dto

// GeneralAssetGroupPolicy defines how a Seedance asset protocol fulfills the
// unified northbound asset_group_id contract for general assets.
type GeneralAssetGroupPolicy string

const (
	GeneralAssetGroupPolicyNone            GeneralAssetGroupPolicy = "none"
	GeneralAssetGroupPolicyDefaultFallback GeneralAssetGroupPolicy = "default_fallback"
	// GeneralAssetGroupPolicyHosted marks the platform-hosted FunCloud path:
	// the caller's optional group ID names a platform-hosted group relation, no
	// Provider group or Channel default group exists, and assets resolve inside
	// the platform instead of upstream.
	GeneralAssetGroupPolicyHosted GeneralAssetGroupPolicy = "hosted"
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
	case AssetUpstreamProtocolFunCloudHosted:
		return GeneralAssetGroupPolicyHosted
	default:
		return ""
	}
}
