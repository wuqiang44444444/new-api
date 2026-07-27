package common

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOpenAIVideoMultipartRequestRecognizesInputReferenceFile(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "sora-2"))
	require.NoError(t, writer.WriteField("prompt", "animate this frame"))
	file, err := writer.CreateFormFile("input_reference", "frame.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body.Bytes()))
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	request, err := parseOpenAIVideoMultipartRequest(context)

	require.NoError(t, err)
	assert.Equal(t, "animate this frame", request.Prompt)
	assert.Equal(t, "multipart://input_reference", request.InputReference)
	assert.True(t, request.HasImage())
}
