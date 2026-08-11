package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type VideoUpstreamProtocol = kitdto.VideoUpstreamProtocol
type AssetUpstreamProtocol = kitdto.AssetUpstreamProtocol

const (
	VideoUpstreamProtocolModelArkV3Volcengine = kitdto.VideoUpstreamProtocolModelArkV3Volcengine
	VideoUpstreamProtocolModelArkV3BytePlus   = kitdto.VideoUpstreamProtocolModelArkV3BytePlus
	VideoUpstreamProtocolMediaTaskV1          = kitdto.VideoUpstreamProtocolMediaTaskV1
	VideoUpstreamProtocolArkMediaV1           = kitdto.VideoUpstreamProtocolArkMediaV1
	VideoUpstreamProtocolURLMediaArraysV1     = kitdto.VideoUpstreamProtocolURLMediaArraysV1
	VideoUpstreamProtocolFunCloudSeedanceV2   = kitdto.VideoUpstreamProtocolFunCloudSeedanceV2

	AssetUpstreamProtocolNone             = kitdto.AssetUpstreamProtocolNone
	AssetUpstreamProtocolVolcengineAction = kitdto.AssetUpstreamProtocolVolcengineAction
	AssetUpstreamProtocolBytePlusAction   = kitdto.AssetUpstreamProtocolBytePlusAction
	AssetUpstreamProtocolArkAssetsV1      = kitdto.AssetUpstreamProtocolArkAssetsV1
	AssetUpstreamProtocolRelayAssetsV1    = kitdto.AssetUpstreamProtocolRelayAssetsV1
)

var ValidateVideoUpstreamProtocol = kitdto.ValidateVideoUpstreamProtocol
var ValidateAssetUpstreamProtocol = kitdto.ValidateAssetUpstreamProtocol
