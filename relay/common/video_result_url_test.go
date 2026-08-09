package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSameOriginVideoResultURLSupportsHTTPAndHTTPS(t *testing.T) {
	for _, test := range []struct {
		name    string
		result  string
		baseURL string
		want    string
	}{
		{
			name:    "https",
			result:  " https://video.example.com:443/v1/a/result.mp4?signature=secret ",
			baseURL: "https://VIDEO.example.com/api",
			want:    "https://video.example.com:443/v1/a/result.mp4?signature=secret",
		},
		{
			name:    "http",
			result:  " http://video.example.com:80/v1/a/result.mp4?signature=secret ",
			baseURL: "http://VIDEO.example.com/api",
			want:    "http://video.example.com:80/v1/a/result.mp4?signature=secret",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := ValidateSameOriginVideoResultURL(test.result, test.baseURL)
			require.NoError(t, err)
			assert.Equal(t, test.want, result)
		})
	}
}

func TestValidateSameOriginVideoResultURLRejectsUnsafeOrDifferentOrigins(t *testing.T) {
	for _, test := range []struct {
		name    string
		result  string
		baseURL string
	}{
		{name: "missing result", baseURL: "http://video.example.com"},
		{name: "relative result", result: "/v1/a/result.mp4", baseURL: "http://video.example.com"},
		{name: "unsupported scheme", result: "ftp://video.example.com/result.mp4", baseURL: "ftp://video.example.com"},
		{name: "scheme mismatch", result: "https://video.example.com/result.mp4", baseURL: "http://video.example.com"},
		{name: "userinfo", result: "http://user@video.example.com/result.mp4", baseURL: "http://video.example.com"},
		{name: "different host", result: "http://cdn.example.com/result.mp4", baseURL: "http://video.example.com"},
		{name: "different port", result: "http://video.example.com:8080/result.mp4", baseURL: "http://video.example.com"},
		{name: "invalid base", result: "http://video.example.com/result.mp4", baseURL: "/relative"},
		{name: "too long", result: "http://video.example.com/" + strings.Repeat("a", maxSameOriginVideoResultURLLength), baseURL: "http://video.example.com"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateSameOriginVideoResultURL(test.result, test.baseURL)
			require.Error(t, err)
		})
	}
}
