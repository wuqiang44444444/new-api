package gemini

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiImageDeliversExactPixelsWithoutDistortion(t *testing.T) {
	var original bytes.Buffer
	require.NoError(t, png.Encode(&original, image.NewNRGBA(image.Rect(0, 0, 32, 18))))
	c := &gin.Context{}
	c.Set(geminiImageSizeKey, "64x36")
	result, err := normalizeGeminiImagePixels(c, GeminiImageResult{MimeType: "image/png", Data: original.Bytes()})
	require.NoError(t, err)
	config, _, err := image.DecodeConfig(bytes.NewReader(result.Data))
	require.NoError(t, err)
	assert.Equal(t, 64, config.Width)
	assert.Equal(t, 36, config.Height)
	c.Set(geminiImageSizeKey, "64x64")
	_, err = normalizeGeminiImagePixels(c, GeminiImageResult{MimeType: "image/png", Data: original.Bytes()})
	require.Error(t, err, "mismatched upstream ratio cannot be stretched or cropped")
}

func TestGeminiImageConverterPreservesContract400(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"}, RelayMode: relayconstant.RelayModeImagesGenerations}
	for _, body := range []string{
		`{"model":"gemini-3.1-flash-image","prompt":"p","n":2}`,
		`{"model":"gemini-3.1-flash-image","prompt":"p","size":"1792x1024"}`,
		`{"model":"gemini-3.1-flash-image","prompt":"p","size":"16:9"}`,
	} {
		request := geminiImageRequest(t, body)
		_, err := (&Adaptor{}).ConvertImageRequest(nil, info, *request)
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
		assert.True(t, types.IsSkipRetryError(apiErr))
	}
}

func TestGeminiImageUsageDoesNotInventZeroCompletion(t *testing.T) {
	body := map[string]any{
		"candidates":    []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "AQID"}}}}}},
		"usageMetadata": map[string]any{"promptTokenCount": 10, "candidatesTokenCount": 0, "totalTokenCount": 10},
	}
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	_, usage, apiErr := ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, encoded)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Zero(t, usage.CompletionTokens, "no fixed 1400-token image estimate")
	delete(body, "usageMetadata")
	encoded, err = common.Marshal(body)
	require.NoError(t, err)
	_, usage, apiErr = ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, encoded)
	require.Nil(t, apiErr)
	assert.Nil(t, usage, "absent metadata must remain absent")
}
