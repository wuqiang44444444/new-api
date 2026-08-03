package controller

import (
	"mime"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoContentProxyCopiesOnlyAllowlistedHeaders(t *testing.T) {
	source := http.Header{
		"Content-Type":        []string{"video/mp4"},
		"Content-Length":      []string{"1024"},
		"Content-Range":       []string{"bytes 0-1023/2048"},
		"Accept-Ranges":       []string{"bytes"},
		"Etag":                []string{`"etag-value"`},
		"Last-Modified":       []string{"Wed, 22 Jul 2026 10:00:00 GMT"},
		"Content-Disposition": []string{`attachment; filename="../private/video.mp4"`},
		"Set-Cookie":          []string{"provider_session=secret"},
		"Authorization":       []string{"Bearer provider-secret"},
		"X-Upstream-Request":  []string{"upstream-request-id"},
	}
	destination := http.Header{}

	copySafeVideoContentHeaders(destination, source)

	assert.Equal(t, "video/mp4", destination.Get("Content-Type"))
	assert.Equal(t, "bytes 0-1023/2048", destination.Get("Content-Range"))
	assert.Equal(t, "", destination.Get("Set-Cookie"))
	assert.Equal(t, "", destination.Get("Authorization"))
	assert.Equal(t, "", destination.Get("X-Upstream-Request"))
	disposition, params, err := mime.ParseMediaType(destination.Get("Content-Disposition"))
	require.NoError(t, err)
	assert.Equal(t, "attachment", disposition)
	assert.Equal(t, "video.mp4", params["filename"])
}

func TestVideoContentProxyRejectsUnsafeDisposition(t *testing.T) {
	assert.Empty(t, safeVideoContentDisposition("form-data; name=provider-secret"))
	assert.Empty(t, safeVideoContentDisposition("attachment; invalid"))
}
