package dto

import (
	"fmt"
	"strings"
)

// VideoUpstreamProtocol identifies a code-backed Seedance southbound adapter.
// It is selected by an administrator; it is not a JSON transformation profile.
type VideoUpstreamProtocol string

const (
	VideoUpstreamProtocolModelArkV3Volcengine VideoUpstreamProtocol = "modelark_v3_volcengine"
	VideoUpstreamProtocolModelArkV3BytePlus   VideoUpstreamProtocol = "modelark_v3_byteplus"
	VideoUpstreamProtocolMediaTaskV1          VideoUpstreamProtocol = "media_task_v1"
	VideoUpstreamProtocolArkMediaV1           VideoUpstreamProtocol = "ark_media_v1"
	VideoUpstreamProtocolURLMediaArraysV1     VideoUpstreamProtocol = "url_media_arrays_v1"
	VideoUpstreamProtocolFunCloudSeedanceV2   VideoUpstreamProtocol = "funcloud_seedance_v2"
)

func (p VideoUpstreamProtocol) IsValid() bool {
	switch p {
	case VideoUpstreamProtocolModelArkV3Volcengine,
		VideoUpstreamProtocolModelArkV3BytePlus,
		VideoUpstreamProtocolMediaTaskV1,
		VideoUpstreamProtocolArkMediaV1,
		VideoUpstreamProtocolURLMediaArraysV1,
		VideoUpstreamProtocolFunCloudSeedanceV2:
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
	case VideoUpstreamProtocolModelArkV3Volcengine, VideoUpstreamProtocolModelArkV3BytePlus:
		return VideoUpstreamProfileOfficial
	case VideoUpstreamProtocolMediaTaskV1:
		return VideoUpstreamProfileThirdPartyRelay
	case VideoUpstreamProtocolArkMediaV1:
		return VideoUpstreamProfileThirdPartyReverseProxy
	case VideoUpstreamProtocolURLMediaArraysV1:
		return VideoUpstreamProfileThirdPartyJSONVideoMediaArrays
	case VideoUpstreamProtocolFunCloudSeedanceV2:
		return VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
	default:
		return ""
	}
}

func (p VideoUpstreamProtocol) TransportPaths(providerModel string) (string, string) {
	switch p {
	case VideoUpstreamProtocolMediaTaskV1:
		return "/v1/media/generations", "/v1/media/tasks/{task_id}"
	case VideoUpstreamProtocolArkMediaV1:
		return "/v1/ark/media/generations", "/v1/ark/media/tasks/{task_id}"
	case VideoUpstreamProtocolURLMediaArraysV1:
		return "/v1/videos", "/v1/videos/{task_id}"
	case VideoUpstreamProtocolFunCloudSeedanceV2:
		createPath := "/api/v2/open/aigc/seedance2-0"
		if strings.Contains(strings.ToLower(strings.TrimSpace(providerModel)), "fast") {
			createPath += "-fast"
		}
		return createPath, "/api/v2/open/aigc/{task_id}"
	default:
		return "", ""
	}
}

type AssetUpstreamProtocol string

const (
	AssetUpstreamProtocolNone             AssetUpstreamProtocol = "none"
	AssetUpstreamProtocolVolcengineAction AssetUpstreamProtocol = "volcengine_assets_action_v2024_01_01"
	AssetUpstreamProtocolBytePlusAction   AssetUpstreamProtocol = "byteplus_assets_action_v2024_01_01"
	AssetUpstreamProtocolArkAssetsV1      AssetUpstreamProtocol = "ark_assets_v1"
	AssetUpstreamProtocolRelayAssetsV1    AssetUpstreamProtocol = "relay_assets_v1"
)

func (p AssetUpstreamProtocol) IsValid() bool {
	switch p {
	case AssetUpstreamProtocolNone,
		AssetUpstreamProtocolVolcengineAction,
		AssetUpstreamProtocolBytePlusAction,
		AssetUpstreamProtocolArkAssetsV1,
		AssetUpstreamProtocolRelayAssetsV1:
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
	case AssetUpstreamProtocolRelayAssetsV1:
		return AssetUpstreamProfileRelay
	default:
		return ""
	}
}
