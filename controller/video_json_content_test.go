package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONVideoContentSourceDefaultsOmniToV2WithoutBearer(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		VideoUpstreamQueryBaseURL: "https://video.example.com/root",
		Key:                       "provider-key",
		UpstreamTaskID:            "provider/task",
		ResultURL:                 "https://video.example.com/v1/a/result.mp4?signature=secret",
	}}

	contentURL, key, handled, err := jsonVideoContentSource(task)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, task.PrivateData.ResultURL, contentURL)
	assert.Empty(t, key)

	task.PrivateData.ResultURL = "http://video.example.com/result.mp4"
	_, _, handled, err = jsonVideoContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.ResultURL = "https://user@video.example.com/result.mp4"
	_, _, handled, err = jsonVideoContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)
}

func TestJSONVideoContentSourceV2UsesValidatedPrivateResultWithoutBearer(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{
		VideoUpstreamProfile:      dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		SouthboundAdapterVersion:  "54:third_party_json_video_omni_reference:v2",
		VideoUpstreamQueryBaseURL: "https://video.example.com/root",
		Key:                       "provider-key",
		UpstreamTaskID:            "provider-task",
		ResultURL:                 "https://video.example.com/v1/a/result.mp4?signature=secret",
	}}

	contentURL, key, handled, err := jsonVideoContentSource(task)
	require.NoError(t, err)
	assert.True(t, handled)
	assert.Equal(t, task.PrivateData.ResultURL, contentURL)
	assert.Empty(t, key)

	task.PrivateData.ResultURL = "https://cdn.example.com/result.mp4"
	_, _, handled, err = jsonVideoContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.SouthboundAdapterVersion = "54:third_party_json_video_omni_reference:v3"
	_, _, handled, err = jsonVideoContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)

	task.PrivateData.SouthboundAdapterVersion = "54:third_party_json_video_omni_reference:v1"
	_, _, handled, err = jsonVideoContentSource(task)
	assert.True(t, handled)
	require.Error(t, err)
}
