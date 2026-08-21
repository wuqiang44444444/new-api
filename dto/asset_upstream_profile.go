package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type AssetUpstreamProfile = kitdto.AssetUpstreamProfile

const (
	AssetUpstreamProfileNone             = kitdto.AssetUpstreamProfileNone
	AssetUpstreamProfileArk              = kitdto.AssetUpstreamProfileArk
	AssetUpstreamProfileRelay            = kitdto.AssetUpstreamProfileRelay
	AssetUpstreamProfileMoxingJoyCreator = kitdto.AssetUpstreamProfileMoxingJoyCreator
	AssetUpstreamProfileMoxingVolc       = kitdto.AssetUpstreamProfileMoxingVolc
	AssetUpstreamProfileOfficial         = kitdto.AssetUpstreamProfileOfficial
	AssetUpstreamProfileFunCloudMaterial = kitdto.AssetUpstreamProfileFunCloudMaterial
	AssetUpstreamProfileCMCCAICCV2       = kitdto.AssetUpstreamProfileCMCCAICCV2
)

var ValidateAssetUpstreamProfile = kitdto.ValidateAssetUpstreamProfile
