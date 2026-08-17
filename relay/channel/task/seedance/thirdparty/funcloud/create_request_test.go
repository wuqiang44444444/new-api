package funcloud

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRequestUsesRichContentAndAcceptsProviderAssetReferences(t *testing.T) {
	falseValue := false
	zero := 0
	payload, err := CreateRequest(&dto.ModelArkVideoCreateRequest{
		Model: "seedance-2-fast",
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("animate")},
			{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "asset://provider-material-1"}},
		},
		Duration: common.GetPointer(5), Resolution: common.GetPointer("720p"),
		GenerateAudio: &falseValue, Watermark: &falseValue, CameraFixed: &falseValue, Seed: &zero,
	})
	require.NoError(t, err)

	var request map[string]any
	require.NoError(t, common.Unmarshal(payload, &request))
	assert.NotContains(t, request, "model")
	assert.NotContains(t, request, "prompt")
	assert.NotContains(t, request, "mode")
	assert.Equal(t, false, request["generateAudio"])
	assert.Equal(t, false, request["watermark"])
	assert.Equal(t, false, request["cameraFixed"])
	assert.Equal(t, float64(0), request["seed"])
	content, ok := request["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	image, ok := content[1].(map[string]any)
	require.True(t, ok)
	imageURL, ok := image["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://provider-material-1", imageURL["url"])
}

func TestCreateRequestPassesOpaqueProviderAssetReferenceWithoutInterpretingIt(t *testing.T) {
	for _, reference := range []string{"asset://nested/id", "asset://contains space"} {
		payload, err := CreateRequest(&dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{{
			Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: reference},
		}}})
		require.NoError(t, err)
		assert.Contains(t, string(payload), reference)
	}
}
