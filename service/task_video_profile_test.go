package service

import (
	"testing"

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
func TestTaskVideoUpstreamQueryConfigPrefersTaskSnapshot(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamQueryPathTemplate: "/channel/tasks/{task_id}"})
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamQueryBaseURL:      "https://snapshot.example.com",
		VideoUpstreamQueryPathTemplate: "/snapshot/tasks/{task_id}",
	}}

	assert.Equal(t, "https://snapshot.example.com", taskVideoUpstreamQueryBaseURL(task, channel))
	assert.Equal(t, "/snapshot/tasks/{task_id}", taskVideoUpstreamQueryPath(task, channel))
}

func TestTaskVideoUpstreamQueryPathDoesNotReadChannelSettings(t *testing.T) {
	channel := &model.Channel{}
	channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamQueryPathTemplate: "/channel/tasks/{task_id}"})

	assert.Empty(t, taskVideoUpstreamQueryPath(&model.Task{}, channel))
	assert.Empty(t, taskVideoUpstreamQueryPath(&model.Task{}, &model.Channel{}))
}
