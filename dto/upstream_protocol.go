package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type VideoUpstreamProtocol = kitdto.VideoUpstreamProtocol
type AssetUpstreamProtocol = kitdto.AssetUpstreamProtocol

const (
	VideoUpstreamProtocolModelArkV3Volcengine = kitdto.VideoUpstreamProtocolModelArkV3Volcengine
	VideoUpstreamProtocolModelArkV3BytePlus   = kitdto.VideoUpstreamProtocolModelArkV3BytePlus
	VideoUpstreamProtocolTokenSaveMediaTaskV1 = kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1
	VideoUpstreamProtocolMoxingMediaTaskV1    = kitdto.VideoUpstreamProtocolMoxingMediaTaskV1
	VideoUpstreamProtocolMoxingModelArkV1     = kitdto.VideoUpstreamProtocolMoxingModelArkV1
	VideoUpstreamProtocolArkMediaV1           = kitdto.VideoUpstreamProtocolArkMediaV1
	VideoUpstreamProtocolFeicaiVideosV1       = kitdto.VideoUpstreamProtocolFeicaiVideosV1
	VideoUpstreamProtocolFunCloudSeedance     = kitdto.VideoUpstreamProtocolFunCloudSeedance

	AssetUpstreamProtocolNone               = kitdto.AssetUpstreamProtocolNone
	AssetUpstreamProtocolVolcengineAction   = kitdto.AssetUpstreamProtocolVolcengineAction
	AssetUpstreamProtocolBytePlusAction     = kitdto.AssetUpstreamProtocolBytePlusAction
	AssetUpstreamProtocolArkAssetsV1        = kitdto.AssetUpstreamProtocolArkAssetsV1
	AssetUpstreamProtocolTokenSaveAssetsV1  = kitdto.AssetUpstreamProtocolTokenSaveAssetsV1
	AssetUpstreamProtocolMoxingJoyCreatorV1 = kitdto.AssetUpstreamProtocolMoxingJoyCreatorV1
	AssetUpstreamProtocolMoxingVolcAssetsV1 = kitdto.AssetUpstreamProtocolMoxingVolcAssetsV1
	AssetUpstreamProtocolFunCloudMaterial   = kitdto.AssetUpstreamProtocolFunCloudMaterial
)

var ValidateVideoUpstreamProtocol = kitdto.ValidateVideoUpstreamProtocol
var ValidateAssetUpstreamProtocol = kitdto.ValidateAssetUpstreamProtocol
