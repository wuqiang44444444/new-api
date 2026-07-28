package openai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTImage2GenerationRequestContractUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const requestJSON = `{
		"model":"gpt-image-2",
		"prompt":"a production image",
		"n":2,
		"size":"1536x1024",
		"quality":"high",
		"response_format":"url",
		"user":"user-ref",
		"extra_fields":{"reference_images":["https://example.com/reference.png"]},
		"background":"transparent",
		"moderation":"low",
		"output_format":"png",
		"output_compression":80,
		"partial_images":2,
		"stream":false,
		"watermark":false
	}`

	var request dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(requestJSON), &request))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(requestJSON))
	c.Request.Header.Set("Content-Type", "application/json")

	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}, request)
	require.NoError(t, err)
	body, err := common.Marshal(converted)
	require.NoError(t, err)

	assert.JSONEq(t, requestJSON, string(body))
}

func TestGPTImage2EditMultipartContractUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "replace the background"))
	require.NoError(t, writer.WriteField("n", "2"))
	require.NoError(t, writer.WriteField("quality", "high"))
	require.NoError(t, writer.WriteField("background", "transparent"))
	for _, content := range []string{"first image", "second image"} {
		part, err := writer.CreateFormFile("image[]", "input.png")
		require.NoError(t, err)
		_, err = part.Write([]byte(content))
		require.NoError(t, err)
	}
	mask, err := writer.CreateFormFile("mask", "mask.png")
	require.NoError(t, err)
	_, err = mask.Write([]byte("mask image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	converted, err := (&Adaptor{}).ConvertImageRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "replace the background",
	})
	require.NoError(t, err)
	convertedBody, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	replayed := httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(convertedBody.Bytes()))
	replayed.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	require.NoError(t, replayed.ParseMultipartForm(32<<20))
	assert.Equal(t, "gpt-image-2", replayed.PostForm.Get("model"))
	assert.Equal(t, "replace the background", replayed.PostForm.Get("prompt"))
	assert.Equal(t, "2", replayed.PostForm.Get("n"))
	assert.Equal(t, "high", replayed.PostForm.Get("quality"))
	assert.Equal(t, "transparent", replayed.PostForm.Get("background"))
	require.Len(t, replayed.MultipartForm.File["image[]"], 2)
	require.Len(t, replayed.MultipartForm.File["mask"], 1)

	for index, expected := range []string{"first image", "second image"} {
		file, err := replayed.MultipartForm.File["image[]"][index].Open()
		require.NoError(t, err)
		content, err := io.ReadAll(file)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		assert.Equal(t, expected, string(content))
	}
	maskFile, err := replayed.MultipartForm.File["mask"][0].Open()
	require.NoError(t, err)
	maskContent, err := io.ReadAll(maskFile)
	require.NoError(t, err)
	require.NoError(t, maskFile.Close())
	assert.Equal(t, "mask image", string(maskContent))
}
