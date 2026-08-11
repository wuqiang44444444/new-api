package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type AssetUpstreamProfile = kitdto.AssetUpstreamProfile

const (
	AssetUpstreamProfileNone     = kitdto.AssetUpstreamProfileNone
	AssetUpstreamProfileArk      = kitdto.AssetUpstreamProfileArk
	AssetUpstreamProfileRelay    = kitdto.AssetUpstreamProfileRelay
	AssetUpstreamProfileOfficial = kitdto.AssetUpstreamProfileOfficial
)

var ValidateAssetUpstreamProfile = kitdto.ValidateAssetUpstreamProfile
