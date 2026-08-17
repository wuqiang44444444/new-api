package dto

import kitdto "github.com/QuantumNous/new-api/relaykit/dto"

type VideoUpstreamProfile = kitdto.VideoUpstreamProfile

const (
	VideoUpstreamProfileOfficial                   = kitdto.VideoUpstreamProfileOfficial
	VideoUpstreamProfileThirdPartyRelay            = kitdto.VideoUpstreamProfileThirdPartyRelay
	VideoUpstreamProfileThirdPartyMoxingModelArk   = kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk
	VideoUpstreamProfileThirdPartyReverseProxy     = kitdto.VideoUpstreamProfileThirdPartyReverseProxy
	VideoUpstreamProfileThirdPartyFeicaiVideos     = kitdto.VideoUpstreamProfileThirdPartyFeicaiVideos
	VideoUpstreamProfileThirdPartyFunCloudSeedance = kitdto.VideoUpstreamProfileThirdPartyFunCloudSeedance
)

var ValidateVideoUpstreamProfile = kitdto.ValidateVideoUpstreamProfile
var ValidateVideoUpstreamURL = kitdto.ValidateVideoUpstreamURL
