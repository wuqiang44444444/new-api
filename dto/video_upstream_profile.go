package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type VideoUpstreamProfile = kitdto.VideoUpstreamProfile

const (
	VideoUpstreamProfileOfficial               = kitdto.VideoUpstreamProfileOfficial
	VideoUpstreamProfileThirdPartyRelay        = kitdto.VideoUpstreamProfileThirdPartyRelay
	VideoUpstreamProfileThirdPartyReverseProxy = kitdto.VideoUpstreamProfileThirdPartyReverseProxy
)

var ValidateVideoUpstreamProfile = kitdto.ValidateVideoUpstreamProfile
var ValidateVideoUpstreamURL = kitdto.ValidateVideoUpstreamURL
