package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type VideoUpstreamProfile = kitdto.VideoUpstreamProfile

const (
	VideoUpstreamProfileOfficial                         = kitdto.VideoUpstreamProfileOfficial
	VideoUpstreamProfileThirdPartyRelay                  = kitdto.VideoUpstreamProfileThirdPartyRelay
	VideoUpstreamProfileThirdPartyReverseProxy           = kitdto.VideoUpstreamProfileThirdPartyReverseProxy
	VideoUpstreamProfileThirdPartyJSONVideoOmniReference = kitdto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference
	VideoUpstreamProfileThirdPartyFunCloudSeedanceV2     = kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
)

var ValidateVideoUpstreamProfile = kitdto.ValidateVideoUpstreamProfile
var ValidateVideoUpstreamURL = kitdto.ValidateVideoUpstreamURL
