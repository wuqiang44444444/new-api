package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteMetadataAssetReferencesOnlyRewritesRegisteredValues(t *testing.T) {
	metadata := map[string]any{
		"content": []any{
			map[string]any{"image_url": map[string]any{"url": "asset://ast_12345678901234567890123456789012"}},
			map[string]any{"video_url": map[string]any{"url": "https://example.com/video.mp4"}},
		},
	}
	rewriteMetadataAssetReferences(metadata, map[string]string{
		"asset://ast_12345678901234567890123456789012": "asset://asset-upstream",
	})

	content, ok := metadata["content"].([]any)
	require.True(t, ok)
	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	image, ok := first["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://asset-upstream", image["url"])
	second, ok := content[1].(map[string]any)
	require.True(t, ok)
	video, ok := second["video_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/video.mp4", video["url"])
}
