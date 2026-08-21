package dto

import (
	"fmt"
)

// VideoUpstreamProtocol identifies a code-backed Seedance southbound adapter.
// It is selected by an administrator; it is not a JSON transformation profile.
type VideoUpstreamProtocol string

const (
	VideoUpstreamProtocolModelArkV3Volcengine VideoUpstreamProtocol = "modelark_v3_volcengine"
	VideoUpstreamProtocolModelArkV3BytePlus   VideoUpstreamProtocol = "modelark_v3_byteplus"
	VideoUpstreamProtocolModelArkV3CMCC       VideoUpstreamProtocol = "modelark_v3_cmcc"
	VideoUpstreamProtocolTokenSaveMediaTaskV1 VideoUpstreamProtocol = "tokensave_media_task_v1"
	VideoUpstreamProtocolMoxingMediaTaskV1    VideoUpstreamProtocol = "moxing_media_task_v1"
	VideoUpstreamProtocolMoxingModelArkV1     VideoUpstreamProtocol = "moxing_modelark_media_v1"
	VideoUpstreamProtocolArkMediaV1           VideoUpstreamProtocol = "ark_media_v1"
	VideoUpstreamProtocolFeicaiVideosV1       VideoUpstreamProtocol = "feicai_videos_v1"
	VideoUpstreamProtocolFunCloudSeedance     VideoUpstreamProtocol = "funcloud_seedance"
)

func (p VideoUpstreamProtocol) IsValid() bool {
	switch p {
	case VideoUpstreamProtocolModelArkV3Volcengine,
		VideoUpstreamProtocolModelArkV3BytePlus,
		VideoUpstreamProtocolModelArkV3CMCC,
		VideoUpstreamProtocolTokenSaveMediaTaskV1,
		VideoUpstreamProtocolMoxingMediaTaskV1,
		VideoUpstreamProtocolMoxingModelArkV1,
		VideoUpstreamProtocolArkMediaV1,
		VideoUpstreamProtocolFeicaiVideosV1,
		VideoUpstreamProtocolFunCloudSeedance:
		return true
	default:
		return false
	}
}

func ValidateVideoUpstreamProtocol(p VideoUpstreamProtocol) error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported Seedance video upstream protocol %q", p)
	}
	return nil
}

// TransportProfile selects the proven request/response transport used by the
// code-backed Seedance adapter. It is not persisted as Seedance configuration.
func (p VideoUpstreamProtocol) TransportProfile() VideoUpstreamProfile {
	switch p {
	case VideoUpstreamProtocolModelArkV3Volcengine,
		VideoUpstreamProtocolModelArkV3BytePlus,
		VideoUpstreamProtocolModelArkV3CMCC:
		return VideoUpstreamProfileOfficial
	case VideoUpstreamProtocolTokenSaveMediaTaskV1, VideoUpstreamProtocolMoxingMediaTaskV1:
		return VideoUpstreamProfileThirdPartyRelay
	case VideoUpstreamProtocolMoxingModelArkV1:
		return VideoUpstreamProfileThirdPartyMoxingModelArk
	case VideoUpstreamProtocolArkMediaV1:
		return VideoUpstreamProfileThirdPartyReverseProxy
	case VideoUpstreamProtocolFeicaiVideosV1:
		return VideoUpstreamProfileThirdPartyFeicaiVideos
	case VideoUpstreamProtocolFunCloudSeedance:
		return VideoUpstreamProfileThirdPartyFunCloudSeedance
	default:
		return ""
	}
}

func (p VideoUpstreamProtocol) TransportPaths(providerModel string) (string, string) {
	switch p {
	case VideoUpstreamProtocolTokenSaveMediaTaskV1,
		VideoUpstreamProtocolMoxingMediaTaskV1,
		VideoUpstreamProtocolMoxingModelArkV1:
		return "/v1/media/generations", "/v1/media/tasks/{task_id}"
	case VideoUpstreamProtocolArkMediaV1:
		return "/v1/ark/media/generations", "/v1/ark/media/tasks/{task_id}"
	case VideoUpstreamProtocolFeicaiVideosV1:
		return "/v1/videos", "/v1/videos/{task_id}"
	case VideoUpstreamProtocolFunCloudSeedance:
		createPath, ok := map[string]string{
			"seedance-2":      "/api/v2/open/aigc/seedance2-0",
			"seedance-2-fast": "/api/v2/open/aigc/seedance2-0-fast",
			"seedance-2-mini": "/api/v2/open/aigc/seedance2-0-mini",
			"seedance-2-5":    "/api/v2/open/aigc/seedance2-5",
		}[providerModel]
		if !ok {
			return "", ""
		}
		return createPath, "/api/v2/open/aigc/{task_id}"
	default:
		return "", ""
	}
}

type AssetUpstreamProtocol string

const (
	AssetUpstreamProtocolNone               AssetUpstreamProtocol = "none"
	AssetUpstreamProtocolVolcengineAction   AssetUpstreamProtocol = "volcengine_assets_action_v2024_01_01"
	AssetUpstreamProtocolBytePlusAction     AssetUpstreamProtocol = "byteplus_assets_action_v2024_01_01"
	AssetUpstreamProtocolArkAssetsV1        AssetUpstreamProtocol = "ark_assets_v1"
	AssetUpstreamProtocolTokenSaveAssetsV1  AssetUpstreamProtocol = "tokensave_assets_v1"
	AssetUpstreamProtocolMoxingJoyCreatorV1 AssetUpstreamProtocol = "moxing_joycreator_assets_v1"
	AssetUpstreamProtocolMoxingVolcAssetsV1 AssetUpstreamProtocol = "moxing_volc_assets_v1"
	AssetUpstreamProtocolFunCloudMaterial   AssetUpstreamProtocol = "funcloud_material"
	AssetUpstreamProtocolCMCCAICCV2         AssetUpstreamProtocol = "cmcc_aicc_assets_v2"
)

func (p AssetUpstreamProtocol) IsValid() bool {
	switch p {
	case AssetUpstreamProtocolNone,
		AssetUpstreamProtocolVolcengineAction,
		AssetUpstreamProtocolBytePlusAction,
		AssetUpstreamProtocolArkAssetsV1,
		AssetUpstreamProtocolTokenSaveAssetsV1,
		AssetUpstreamProtocolMoxingJoyCreatorV1,
		AssetUpstreamProtocolMoxingVolcAssetsV1,
		AssetUpstreamProtocolFunCloudMaterial,
		AssetUpstreamProtocolCMCCAICCV2:
		return true
	default:
		return false
	}
}

func ValidateAssetUpstreamProtocol(p AssetUpstreamProtocol) error {
	if !p.IsValid() {
		return fmt.Errorf("unsupported Seedance asset upstream protocol %q", p)
	}
	return nil
}

// TransportProfile selects the asset transport implementation. It is not a
// second administrator-facing protocol or a compatibility fallback.
func (p AssetUpstreamProtocol) TransportProfile() AssetUpstreamProfile {
	switch p {
	case AssetUpstreamProtocolNone:
		return AssetUpstreamProfileNone
	case AssetUpstreamProtocolVolcengineAction, AssetUpstreamProtocolBytePlusAction:
		return AssetUpstreamProfileOfficial
	case AssetUpstreamProtocolArkAssetsV1:
		return AssetUpstreamProfileArk
	case AssetUpstreamProtocolTokenSaveAssetsV1:
		return AssetUpstreamProfileRelay
	case AssetUpstreamProtocolMoxingJoyCreatorV1:
		return AssetUpstreamProfileMoxingJoyCreator
	case AssetUpstreamProtocolMoxingVolcAssetsV1:
		return AssetUpstreamProfileMoxingVolc
	case AssetUpstreamProtocolFunCloudMaterial:
		return AssetUpstreamProfileFunCloudMaterial
	case AssetUpstreamProtocolCMCCAICCV2:
		return AssetUpstreamProfileCMCCAICCV2
	default:
		return ""
	}
}
