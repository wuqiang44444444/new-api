package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskProviderChannelDoesNotReadMutableChannelForFrozenTask(t *testing.T) {
	task := &model.Task{
		ChannelId:      999999,
		Platform:       constant.TaskPlatform("24"),
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		PrivateData: model.TaskPrivateData{
			Key:                       "frozen-key",
			VideoUpstreamQueryBaseURL: "https://frozen.example",
			VideoUpstreamProxy:        "http://frozen-proxy.example",
		},
	}

	channel, err := videoTaskProviderChannel(task)

	require.NoError(t, err)
	assert.Equal(t, "frozen-key", channel.Key)
	assert.Equal(t, "https://frozen.example", channel.GetBaseURL())
	assert.Equal(t, "http://frozen-proxy.example", channel.GetSetting().Proxy)
}

func TestGetVertexTaskKeyDoesNotFallBackForFrozenTask(t *testing.T) {
	channel := &model.Channel{Key: "mutated-channel-key"}
	task := &model.Task{ClientProtocol: model.TaskClientProtocolModelArkV3}

	assert.Empty(t, getVertexTaskKey(channel, task))
}
