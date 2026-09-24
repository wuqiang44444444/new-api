package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMiniMaxPublishedMetadataMatchesArtifact(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.MinimaxSource(), jsplugin.Options{}, jsplugin.MinimaxHostContract())
	require.NoError(t, err)
	api, err := minimaxPublicVideoAPI(info.Configuration, "custom-model", "MiniMax-H3", false)
	require.NoError(t, err)
	assert.Equal(t, "modelark_v3", api.Protocol)
	assert.Equal(t, "/api/v3/contents/generations/tasks", api.Creation.Path)
	assert.Equal(t, "custom-model", api.Creation.Model)
	require.Len(t, api.Creation.ContentTypes, 4)
	assert.Equal(t, 9, api.Creation.ContentTypes[1].MaxItems)
	assert.Equal(t, []string{"reference_image", "first_frame", "last_frame"}, api.Creation.ContentTypes[1].Roles)
	assert.Equal(t, "video_url", api.Creation.ContentTypes[2].Type)
	assert.Equal(t, []string{"reference_video"}, api.Creation.ContentTypes[2].Roles)
	assert.Equal(t, 3, api.Creation.ContentTypes[2].MaxItems)
	assert.Equal(t, "audio_url", api.Creation.ContentTypes[3].Type)
	assert.Equal(t, 3, api.Creation.ContentTypes[3].MaxItems)
	for _, p := range api.Creation.Parameters {
		switch p.Name {
		case "content":
			assert.Equal(t, "object", p.ItemType)
			assert.Equal(t, common.GetPointer(16), p.MaxItems)
		case "duration":
			assert.Equal(t, common.GetPointer(4), p.Minimum)
			assert.Equal(t, common.GetPointer(15), p.Maximum)
			assert.Equal(t, 6, p.DefaultValue)
		case "resolution":
			assert.Equal(t, []string{"768p", "2k"}, p.Enum)
			assert.Equal(t, "768p", p.DefaultValue)
		case "ratio":
			assert.Equal(t, "16:9", p.DefaultValue)
		case "content[].text":
			assert.Equal(t, common.GetPointer(7000), p.MaxLength)
		}
		assert.NotContains(t, []string{"generate_audio", "prompt_optimizer", "seed", "frames", "callback_url"}, p.Name)
	}
	raw, err := common.Marshal(api)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "MiniMax-H3")
	assert.NotContains(t, string(raw), constant.VideoUpstreamProtocolJdCloudTaskV1)
}

func TestMiniMaxOriginalDeclarationKeepsTextOnlyScope(t *testing.T) {
	configuration := &jsplugin.SeedanceChannelConfiguration{Videos: []jsplugin.SeedanceVideoConfiguration{{
		Protocol:      constant.VideoUpstreamProtocolJdCloudTaskV1,
		ModelMetadata: map[string]jsplugin.SeedanceVideoModelMetadata{"MiniMax-H3": {DefaultDuration: 6, MinDuration: 6, MaxDuration: 6, Resolutions: []string{"768p"}, Ratios: []string{"16:9"}, DeleteVideo: common.GetPointer(false)}},
	}}}
	api, err := minimaxPublicVideoAPI(configuration, "custom-model", "MiniMax-H3", false)
	require.NoError(t, err)
	require.Len(t, api.Creation.ContentTypes, 1)
	assert.Equal(t, "text", api.Creation.ContentTypes[0].Type)
	for _, p := range api.Creation.Parameters {
		if p.Name == "content" {
			assert.Equal(t, common.GetPointer(1), p.MaxItems)
		}
	}
}

func TestMiniMaxImageAudioDeclarationDoesNotGainFramesOrVideos(t *testing.T) {
	_, info, err := jsplugin.CompileSeedanceExtension(plugins.MinimaxSource(), jsplugin.Options{}, jsplugin.MinimaxHostContract())
	require.NoError(t, err)
	metadata := info.Configuration.Videos[0].ModelMetadata["MiniMax-H3"]
	metadata.AllowFrameImages = false
	metadata.AllowVideos, metadata.MaxVideos = false, 0
	info.Configuration.Videos[0].ModelMetadata["MiniMax-H3"] = metadata
	api, err := minimaxPublicVideoAPI(info.Configuration, "custom-model", "MiniMax-H3", false)
	require.NoError(t, err)
	require.Len(t, api.Creation.ContentTypes, 3)
	assert.Equal(t, []string{"reference_image"}, api.Creation.ContentTypes[1].Roles)
	assert.Equal(t, "audio_url", api.Creation.ContentTypes[2].Type)
	for _, p := range api.Creation.Parameters {
		if p.Name == "content" {
			assert.Equal(t, common.GetPointer(13), p.MaxItems)
		}
	}
}
