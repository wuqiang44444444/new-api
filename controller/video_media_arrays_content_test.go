package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoMediaArraysContentSourceUsesFrozenBearerAndIdentity(t *testing.T) {
	implementation, ok := model.ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
	})
	require.True(t, ok)
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		SouthboundAdapterVersion:  "54:third_party_json_video_media_arrays:v2",
		LinkImplementationID:      implementation.ID,
		LinkImplementationVersion: implementation.Version,
		LinkImplementationHash:    implementation.ContentHash,
		VideoUpstreamQueryBaseURL: "https://video.example.com/root",
		Key:                       "frozen-provider-key",
		ResultURL:                 "https://video.example.com/v1/videos/task/content",
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
	task.PrivateData.ResultURL = "https://cdn.example.com/result.mp4"
	_, _, handled, err = videoMediaArraysContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.ResultURL = "https://video.example.com/result.mp4"
	task.PrivateData.LinkImplementationHash = "sha256:stale"
	_, _, handled, err = videoMediaArraysContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)
}
