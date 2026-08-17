package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type funCloudSourceRoundTripper func(*http.Request) (*http.Response, error)

func (f funCloudSourceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestOpenFunCloudAssetSourceEnforcesMIMEAndSize(t *testing.T) {
	client := &http.Client{Transport: funCloudSourceRoundTripper(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "image/jpeg,image/png,image/webp,image/bmp,image/tiff,image/gif", req.Header.Get("Accept"))
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png; charset=binary"}},
			Body:          io.NopCloser(strings.NewReader("png-data")),
			ContentLength: 8,
		}, nil
	})}
	source, err := openFunCloudAssetSourceWithClient(context.Background(), "https://source.example/image", "image", client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = source.Body.Close() })
	assert.Equal(t, "image/png", source.ContentType)
	assert.Equal(t, "upload.png", source.Filename)
	assert.Equal(t, int64(8), source.Size)

	oversized := &http.Client{Transport: funCloudSourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"video/mp4"}},
			Body:       io.NopCloser(strings.NewReader("ignored")), ContentLength: funCloudMaterialMaxBytes + 1,
		}, nil
	})}
	_, err = openFunCloudAssetSourceWithClient(context.Background(), "https://source.example/video", "video", oversized)
	require.ErrorContains(t, err, "exceeds configured upload limit")

	mismatched := &http.Client{Transport: funCloudSourceRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader("audio")), ContentLength: 5,
		}, nil
	})}
	_, err = openFunCloudAssetSourceWithClient(context.Background(), "https://source.example/audio", "image", mismatched)
	require.ErrorContains(t, err, "MIME does not match")
}

func TestOpenFunCloudAssetSourceRejectsNonHTTPSBeforeFetch(t *testing.T) {
	_, err := openFunCloudAssetSource(context.Background(), "http://example.com:443/source.png", "image")
	require.ErrorIs(t, err, ErrInvalidAssetRequest)
}
