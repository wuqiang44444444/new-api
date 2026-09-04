package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskVideoUpstreamProfilePrefersTaskSnapshot(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay})
	task := &model.Task{PrivateData: model.TaskPrivateData{VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyReverseProxy}}

	assert.Equal(t, dto.VideoUpstreamProfileThirdPartyReverseProxy, taskVideoUpstreamProfile(task, channel))
}

func TestTaskVideoUpstreamProfileDoesNotReadChannelProfile(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay})

	assert.Equal(t, dto.VideoUpstreamProfileOfficial, taskVideoUpstreamProfile(&model.Task{}, channel))
	assert.Equal(t, dto.VideoUpstreamProfileOfficial, taskVideoUpstreamProfile(&model.Task{}, &model.Channel{}))
}

// TestTaskVideoUpstreamQueryConfigPrefersTaskSnapshot 验证查询根地址与路径模板优先任务快照（方案 §7）。
func TestTaskVideoUpstreamQueryBaseURLPrefersTaskSnapshot(t *testing.T) {
	channel := &model.Channel{BaseURL: common.GetPointer("https://channel.example.com")}
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamQueryBaseURL: "https://snapshot.example.com",
	}}

	// Creation-time snapshot wins over channel and type defaults.
	assert.Equal(t, "https://snapshot.example.com", taskVideoUpstreamQueryBaseURL(task, channel, "https://type.default"))

	// Channel base url beats the channel-type default.
	assert.Equal(t, "https://channel.example.com", taskVideoUpstreamQueryBaseURL(&model.Task{}, channel, "https://type.default"))

	// Type default is the last fallback, matching the shared polling flow.
	assert.Equal(t, "https://type.default", taskVideoUpstreamQueryBaseURL(&model.Task{}, &model.Channel{}, "https://type.default"))
}
