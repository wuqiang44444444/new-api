package thirdparty

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// Exercise the published artifact, including model validation, rather than the
// retired Go request converter. Response normalization remains covered separately.
func tokenSavePluginCreateRequest(t *testing.T, body []byte) ([]byte, error) {
	t.Helper()
	plugin, _, err := jsplugin.CompileSeedanceExtension(plugins.SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	request := decodeObject(t, body)
	result, err := plugin.Engine.CallPath(t.Context(), "seedance", []string{"tokensave_media_task_v1", "buildCreate"}, map[string]any{
		"protocol": "tokensave_media_task_v1", "providerModel": request["model"], "request": request,
	})
	if err != nil {
		return nil, err
	}
	object, ok := result.(map[string]any)
	require.True(t, ok)
	return common.Marshal(object["body"])
}

func TestTokenSavePluginCreateRequestUsesMediaTaskFieldNamesAndPreservesExplicitZero(t *testing.T) {
	body, err := tokenSavePluginCreateRequest(t, []byte(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"hello"}],"duration":5,"seed":0,"generate_audio":false,"ratio":"16:9"}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "video_generation", result["capability"])
	assert.Equal(t, "text", result["input_mode"])
	assert.Equal(t, "none", result["control_mode"])
	assert.Equal(t, float64(5), result["duration_seconds"])
	assert.Equal(t, float64(0), result["seed"])
	assert.Equal(t, false, result["with_audio"])
	assert.Equal(t, "16:9", result["aspect_ratio"])
	assert.NotContains(t, result, "generate_audio")
	assert.NotContains(t, result, "ratio")
}

func TestTokenSavePluginCreateRequestUsesCurrentDoubaoSeedanceContract(t *testing.T) {
	body, err := tokenSavePluginCreateRequest(t, []byte(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"snow mountain"}],"duration":5,"resolution":"720p","ratio":"16:9","generate_audio":false}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "doubao-seedance-2-0-260128", result["model"])
	assert.Equal(t, "video_generation", result["capability"])
	assert.Equal(t, "text", result["input_mode"])
	assert.Equal(t, "none", result["control_mode"])
	assert.Equal(t, "snow mountain", result["prompt"])
	assert.Equal(t, float64(5), result["duration_seconds"])
	assert.Equal(t, "720p", result["resolution"])
	assert.Equal(t, "16:9", result["aspect_ratio"])
	assert.Equal(t, false, result["with_audio"])
}

func TestTokenSavePluginCreateRequestDoesNotSilentlyDropOptionalFields(t *testing.T) {
	body, err := tokenSavePluginCreateRequest(t, []byte(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"hello"}],"seed":0,"camera_fixed":false,"watermark":false}`))
	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, float64(0), result["seed"])
	assert.Equal(t, false, result["camera_fixed"])
	assert.Equal(t, false, result["watermark"])
}

func TestTokenSavePluginCreateRequestMapsFrameControls(t *testing.T) {
	body, err := tokenSavePluginCreateRequest(t, []byte(`{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","role":"first_frame","image_url":{"url":"https://cdn.example/first.png"}},{"type":"image_url","role":"last_frame","image_url":{"url":"https://cdn.example/last.png"}},{"type":"text","text":"move"}]}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "single_image", result["input_mode"])
	assert.Equal(t, "end_frame", result["control_mode"])
	assert.Equal(t, "https://cdn.example/first.png", result["image"])
	assert.Equal(t, "https://cdn.example/last.png", result["end_image"])
}

func TestTokenSavePluginCreateRequestRejectsUnsupportedInputs(t *testing.T) {
	tests := []string{
		`{"model":"seedance-2-0-oversea","content":[{"type":"text","text":"hello"}]}`,
		`{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"hello"}],"duration":0}`,
		`{"model":"doubao-seedance-2-0-260128","content":[{"type":"image_url","image_url":{"url":"https://cdn.example/one.png"}},{"type":"image_url","image_url":{"url":"https://cdn.example/two.png"}}]}`,
	}
	for _, body := range tests {
		_, err := tokenSavePluginCreateRequest(t, []byte(body))
		require.Error(t, err)
	}
}

func TestTokenSavePluginCreateRequestPreservesReferenceAudioAndVideo(t *testing.T) {
	body, err := tokenSavePluginCreateRequest(t, []byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"follow the references"},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example/reference.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://cdn.example/reference.mp3"}}
		]
	}`))

	require.NoError(t, err)
	result := decodeObject(t, body)
	assert.Equal(t, "multi_image", result["input_mode"])
	assert.Equal(t, "reference", result["control_mode"])
	assert.Equal(t, []any{"https://cdn.example/reference.mp4"}, result["reference_videos"])
	assert.Equal(t, []any{"https://cdn.example/reference.mp3"}, result["reference_audios"])
}
