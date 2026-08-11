package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoMediaArraysContentSourceUsesFrozenBearerAndIdentity(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		SouthboundAdapterVersion:  "61:third_party_json_video_media_arrays:v2",
		VideoUpstreamQueryBaseURL: "http://video.example.com/root",
		Key:                       "frozen-provider-key",
		ResultURL:                 "http://video.example.com/v1/videos/task/content",
	}}

	contentURL, key, handled, err := videoMediaArraysContentSource(task)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, task.PrivateData.ResultURL, contentURL)
	assert.Equal(t, "frozen-provider-key", key)

	task.PrivateData.Key = ""
	_, _, handled, err = videoMediaArraysContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.Key = "frozen-provider-key"
	task.PrivateData.ResultURL = "http://cdn.example.com/result.mp4"
	_, _, handled, err = videoMediaArraysContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

}
