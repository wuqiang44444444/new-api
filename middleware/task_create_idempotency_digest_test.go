package middleware

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

func TestTaskCreateRequestHashCanonicalizesJSON(t *testing.T) {
	first := taskCreateDigestContext(t, "application/json", []byte(`{"model":"m","prompt":"p","seed":9007199254740993}`))
	second := taskCreateDigestContext(t, "application/json", []byte(`{"seed":9007199254740993,"prompt":"p","model":"m"}`))

	firstHash, err := taskCreateRequestHash(first, "openai_videos")
	require.NoError(t, err)
	secondHash, err := taskCreateRequestHash(second, "openai_videos")
	require.NoError(t, err)

	assert.Equal(t, firstHash, secondHash)

	different := taskCreateDigestContext(t, "application/json", []byte(`{"seed":9007199254740992,"prompt":"p","model":"m"}`))
	differentHash, err := taskCreateRequestHash(different, "openai_videos")
	require.NoError(t, err)
	assert.NotEqual(t, firstHash, differentHash)
}

func TestTaskCreateRequestHashIgnoresMultipartBoundary(t *testing.T) {
	firstType, firstBody := taskCreateMultipartBody(t, "boundary-one")
	secondType, secondBody := taskCreateMultipartBody(t, "boundary-two")
	first := taskCreateDigestContext(t, firstType, firstBody)
	second := taskCreateDigestContext(t, secondType, secondBody)

	firstHash, err := taskCreateRequestHash(first, "openai_videos")
	require.NoError(t, err)
	secondHash, err := taskCreateRequestHash(second, "openai_videos")
	require.NoError(t, err)

	assert.Equal(t, firstHash, secondHash)
}

func taskCreateDigestContext(t *testing.T, contentType string, body []byte) *gin.Context {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", contentType)
	return context
}

func taskCreateMultipartBody(t *testing.T, boundary string) (string, []byte) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.SetBoundary(boundary))
	require.NoError(t, writer.WriteField("model", "sora-2"))
	require.NoError(t, writer.WriteField("prompt", "video"))
	file, err := writer.CreateFormFile("input_reference", "frame.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("same-image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return writer.FormDataContentType(), body.Bytes()
}
