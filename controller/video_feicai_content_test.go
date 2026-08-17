package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoFeicaiContentSourceUsesFrozenSnapshot(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
		SouthboundAdapterVersion:  "61:third_party_feicai_videos:v1",
		VideoUpstreamQueryBaseURL: "https://video.example.com",
		Key:                       "frozen-provider-key",
		ResultURL:                 "https://video.example.com/results/task.mp4",
	}}

	contentURL, key, handled, err := videoFeicaiContentSource(task)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, task.PrivateData.ResultURL, contentURL)
	assert.Equal(t, "frozen-provider-key", key)

	task.PrivateData.SouthboundAdapterVersion = "61:third_party_feicai_videos:v2"
	contentURL, key, handled, err = videoFeicaiContentSource(task)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, task.PrivateData.ResultURL, contentURL)
	assert.Equal(t, "frozen-provider-key", key)

	task.PrivateData.Key = ""
	_, _, handled, err = videoFeicaiContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.Key = "frozen-provider-key"
	task.PrivateData.ResultURL = "https://cdn.example.com/result.mp4"
	_, _, handled, err = videoFeicaiContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)
}
