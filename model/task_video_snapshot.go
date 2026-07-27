package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
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
	privateData.VideoUpstreamProxy = channel.ChannelSetting.Proxy
	privateData.Key = channel.ApiKey
}

func TaskUsesFrozenVideoConnection(task *Task) bool {
	if task == nil {
		return false
	}
	return task.ClientProtocol == TaskClientProtocolOpenAIVideos ||
		task.ClientProtocol == TaskClientProtocolModelArkV3
}

// FrozenVideoTaskChannel reconstructs the provider connection selected when a
// northbound video task was submitted. It deliberately does not consult the
// mutable channel row.
func FrozenVideoTaskChannel(task *Task) (*Channel, bool) {
	if !TaskUsesFrozenVideoConnection(task) {
		return nil, false
	}
	channelType, err := strconv.Atoi(strings.TrimSpace(string(task.Platform)))
	if err != nil || channelType <= 0 {
		return nil, false
	}
	baseURL := strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL)
	key := strings.TrimSpace(task.PrivateData.Key)
	if baseURL == "" || key == "" {
		return nil, false
	}
	channel := &Channel{
		Id:      task.ChannelId,
		Type:    channelType,
		Key:     key,
		BaseURL: common.GetPointer(baseURL),
		Status:  common.ChannelStatusEnabled,
	}
	channel.SetSetting(dto.ChannelSettings{Proxy: task.PrivateData.VideoUpstreamProxy})
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:           task.PrivateData.VideoUpstreamProfile,
		VideoUpstreamQueryPathTemplate: task.PrivateData.VideoUpstreamQueryPathTemplate,
	})
	return channel, true
}
