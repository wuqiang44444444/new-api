package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// freezeTaskVideoUpstream keeps in-flight Doubao video tasks bound to the
// protocol, endpoint, and selected key used when they were created.
func freezeTaskVideoUpstream(privateData *TaskPrivateData, channel *relaycommon.ChannelMeta) {
	if channel.ChannelType != constant.ChannelTypeDoubaoVideo {
		return
	}
	privateData.VideoUpstreamProfile = channel.ChannelOtherSettings.VideoUpstreamProfile
	if privateData.VideoUpstreamProfile == "" {
		privateData.VideoUpstreamProfile = dto.VideoUpstreamProfileOfficial
	}
	privateData.VideoUpstreamQueryBaseURL = channel.ChannelBaseUrl
	privateData.VideoUpstreamQueryPathTemplate = channel.ChannelOtherSettings.VideoUpstreamQueryPathTemplate
	privateData.Key = channel.ApiKey
}
