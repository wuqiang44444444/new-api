package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactTaskResponseForLogRemovesURLsCredentialsAndMedia(t *testing.T) {
	redacted := string(redactTaskResponseForLog([]byte(`{
		"content":{"video_url":"https://example.com/video.mp4?signature=secret"},
		"nested":[{"url":"https://signed.example/input","authorization":"Bearer secret","cookie":"session=secret","image_base64":"very-long"}],
		"status":"succeeded"
	}`)))

	assert.Contains(t, redacted, `"status":"succeeded"`)
	assert.NotContains(t, redacted, "example.com")
	assert.NotContains(t, redacted, "Bearer secret")
	assert.NotContains(t, redacted, "session=secret")
	assert.NotContains(t, redacted, "very-long")
	assert.Contains(t, redacted, "[redacted]")
}
