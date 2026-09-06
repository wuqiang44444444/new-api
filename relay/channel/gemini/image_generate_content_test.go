package gemini

import (
	"encoding/base64"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func geminiImageRequest(t *testing.T, body string) *dto.ImageRequest {
	t.Helper()
	request := &dto.ImageRequest{}
	require.NoError(t, common.Unmarshal([]byte(body), request))
	return request
}

func TestParseGeminiImageContractRejectsNAndUnpublishedFields(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	request := geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","n":2}`)
	_, apiErr := ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "n must be 1")

	request = geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","quality":"high"}`)
	_, apiErr = ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "quality")

	request = geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","stream":true}`)
	_, apiErr = ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "stream")

	// 显式 stream=false 合法（P14），空 quality 视为未设置（E6）。
	request = geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","stream":false,"quality":""}`)
	contract, apiErr := ParseGeminiImageContract(nil, info, request)
	require.Nil(t, apiErr)
	assert.Equal(t, "b64_json", contract.ResponseFormat)
}

func TestBuildGenerateContentImageRequestGenerations(t *testing.T) {
	contract := &GeminiImageContract{
		Operation: "generations",
		Model:     "gemini-3.1-flash-image",
		Prompt:    "a cat",
		Size:      "1920x1080",
	}
	request, err := BuildGenerateContentImageRequest(contract)
	require.NoError(t, err)
	require.Len(t, request.Contents, 1)
	assert.Equal(t, "user", request.Contents[0].Role)
	require.Len(t, request.Contents[0].Parts, 1)
	assert.Equal(t, "a cat", request.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"TEXT", "IMAGE"}, request.GenerationConfig.ResponseModalities)

	var imageConfig map[string]any
	require.NoError(t, common.Unmarshal(request.GenerationConfig.ImageConfig, &imageConfig))
	assert.Equal(t, "16:9", imageConfig["aspectRatio"])
	assert.Equal(t, "2K", imageConfig["imageSize"])
}

func TestBuildGenerateContentImageRequestEditsMixedInputs(t *testing.T) {
	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47}
	contract := &GeminiImageContract{
		Operation: "edits",
		Model:     "gemini-3.1-flash-image",
		Prompt:    "edit",
		Size:      "auto",
		Images: []service.ImageContractInput{
			{MimeType: "image/png", Data: pngBytes},
			{URL: "https://example.com/ref.png"},
		},
	}
	request, err := BuildGenerateContentImageRequest(contract)
	require.NoError(t, err)
	parts := request.Contents[0].Parts
	require.Len(t, parts, 3)
	assert.Equal(t, "edit", parts[0].Text)
	require.NotNil(t, parts[1].InlineData)
	assert.Equal(t, base64.StdEncoding.EncodeToString(pngBytes), parts[1].InlineData.Data)
	require.NotNil(t, parts[2].FileData)
	assert.Equal(t, "https://example.com/ref.png", parts[2].FileData.FileUri)
	// size=auto：不发送 imageConfig（P4，Provider 默认）。
	assert.Nil(t, request.GenerationConfig.ImageConfig)
}

func TestSizeToGeminiImageConfig(t *testing.T) {
	tests := []struct {
		size        string
		aspectRatio string
		imageSize   string
		ok          bool
	}{
		{"", "", "", false},
		{"auto", "", "", false},
		{"4:3", "", "", false},
		{"1792x1024", "", "", false},
		{"1024x1024", "1:1", "1K", true},
		{"1920x1080", "16:9", "2K", true},
		{"1080x1920", "9:16", "2K", true},
		{"3840x2160", "16:9", "4K", true},
		{"1280x720", "16:9", "1K", true},
	}
	for _, tc := range tests {
		aspectRatio, imageSize, ok := SizeToGeminiImageConfig(tc.size)
		assert.Equal(t, tc.ok, ok, "size=%s", tc.size)
		if tc.ok {
			assert.Equal(t, tc.aspectRatio, aspectRatio, "size=%s", tc.size)
			assert.Equal(t, tc.imageSize, imageSize, "size=%s", tc.size)
		}
	}
}

func TestParseGenerateContentImageResponseBodyFiltersThoughts(t *testing.T) {
	body := `{
		"candidates": [{
			"content": {"parts": [
				{"text":"thinking...","thought":true},
				{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) + `"}},
				{"text":"final commentary"}
			]},
			"finishReason": "STOP"
		}],
		"usageMetadata": {"promptTokenCount": 100, "candidatesTokenCount": 2840, "totalTokenCount": 2940}
	}`
	results, usage, apiErr := ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, []byte(body))
	require.Nil(t, apiErr)
	require.Len(t, results, 1)
	assert.Equal(t, "image/png", results[0].MimeType)
	assert.Equal(t, []byte{1, 2, 3}, results[0].Data)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 2940, usage.TotalTokens)
}

func TestParseGenerateContentImageResponseBodySafetyBlock(t *testing.T) {
	body := `{"candidates":[{"content":{"parts":[]},"finishReason":"SAFETY"}]}`
	_, _, apiErr := ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, []byte(body))
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "safety")

	body = `{"promptFeedback":{"blockReason":"SAFETY"}}`
	_, _, apiErr = ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, []byte(body))
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "blocked")
}

func TestParseGenerateContentImageResponseBodyEmpty(t *testing.T) {
	body := `{"candidates":[{"content":{"parts":[{"text":"no image"}]}}]}`
	_, _, apiErr := ParseGenerateContentImageResponseBody(nil, &relaycommon.RelayInfo{}, []byte(body))
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "no final image")
}

func TestGeminiImageContractRejectsInvalidSize(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations}

	// 非法 size 显式 400，不静默回退 Provider 默认（评审 S10/E6）。
	request := geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","size":"big"}`)
	_, apiErr := ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "size must be auto")

	request = geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","size":"1024x"}`)
	_, apiErr = ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)

	request = geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","size":"a:b"}`)
	_, apiErr = ParseGeminiImageContract(nil, info, request)
	require.NotNil(t, apiErr)

	for _, valid := range []string{"", "auto", "1024x1024", "1920X1080"} {
		request := geminiImageRequest(t, `{"model":"gemini-3.1-flash-image","prompt":"p","size":"`+valid+`"}`)
		_, apiErr := ParseGeminiImageContract(nil, info, request)
		assert.Nil(t, apiErr, "size=%q must be accepted", valid)
	}
}
