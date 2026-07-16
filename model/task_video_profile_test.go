package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestInitTaskFreezesDoubaoVideoProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile dto.VideoUpstreamProfile
		want    dto.VideoUpstreamProfile
	}{
		{name: "empty becomes explicit official", want: dto.VideoUpstreamProfileOfficial},
		{name: "reverse proxy", profile: dto.VideoUpstreamProfileThirdPartyReverseProxy, want: dto.VideoUpstreamProfileThirdPartyReverseProxy},
		{name: "relay", profile: dto.VideoUpstreamProfileThirdPartyRelay, want: dto.VideoUpstreamProfileThirdPartyRelay},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeDoubaoVideo,
					ChannelOtherSettings: dto.ChannelOtherSettings{
						VideoUpstreamProfile: test.profile,
					},
				},
			}

			task := InitTask(constant.TaskPlatform("doubao-video"), info)

			assert.Equal(t, test.want, task.PrivateData.VideoUpstreamProfile)
		})
	}
}

// TestInitTaskFreezesDoubaoVideoQuerySnapshot 验证第三方协议下查询根地址与路径模板被冻结为快照（方案 §7）。
func TestInitTaskFreezesDoubaoVideoQuerySnapshot(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeDoubaoVideo,
			ChannelBaseUrl: "https://example.com",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile:           dto.VideoUpstreamProfileThirdPartyRelay,
				VideoUpstreamCreatePath:        "/v1/media/generations",
				VideoUpstreamQueryPathTemplate: "/v1/media/tasks/{task_id}",
			},
		},
	}

	task := InitTask(constant.TaskPlatform("doubao-video"), info)

	assert.Equal(t, "https://example.com", task.PrivateData.VideoUpstreamQueryBaseURL)
	assert.Equal(t, "/v1/media/tasks/{task_id}", task.PrivateData.VideoUpstreamQueryPathTemplate)
}

// TestInitTaskFreezesDoubaoVideoSelectedKey 验证创建时 distributor 选中的单个 Key 被冻结到任务快照（方案 §15.4 P1-2）。
// 轮询优先读取 private_data.key，从而单 Key 渠道换账号不污染在途任务、多 Key 渠道不再把整组 Key 当作 Bearer。
func TestInitTaskFreezesDoubaoVideoSelectedKey(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeDoubaoVideo,
			ApiKey:      "selected-single-key",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay,
			},
		},
	}

	task := InitTask(constant.TaskPlatform("doubao-video"), info)

	assert.Equal(t, "selected-single-key", task.PrivateData.Key)
}

// TestInitTaskDoesNotFreezeKeyForChannelsWithoutKeySnapshot 确保非 Gemini/Vertex/DoubaoVideo 渠道
// 不冻结 Key，避免回归：Kling 等渠道仍沿用渠道当前 Key，不引入无谓的凭证快照。
func TestInitTaskDoesNotFreezeKeyForChannelsWithoutKeySnapshot(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeKling,
			ApiKey:      "should-not-freeze",
		},
	}

	task := InitTask(constant.TaskPlatform("kling"), info)

	assert.Empty(t, task.PrivateData.Key)
}

func TestInitTaskDoesNotStoreVideoProfileForOtherChannels(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeKling,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay,
			},
		},
	}

	task := InitTask(constant.TaskPlatform("kling"), info)

	assert.Empty(t, task.PrivateData.VideoUpstreamProfile)
}
