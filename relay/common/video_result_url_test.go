package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSameOriginHTTPSVideoResultURL(t *testing.T) {
	result, err := ValidateSameOriginHTTPSVideoResultURL(
		" https://video.example.com:443/v1/a/result.mp4?signature=secret ",
		"https://VIDEO.example.com/api",
	)
	require.NoError(t, err)
	assert.Equal(t, "https://video.example.com:443/v1/a/result.mp4?signature=secret", result)

	for _, candidate := range []string{
		"",
		"/v1/a/result.mp4",
		"http://video.example.com/v1/a/result.mp4",
		"https://user@video.example.com/v1/a/result.mp4",
		"https://cdn.example.com/v1/a/result.mp4",
		"https://video.example.com:444/v1/a/result.mp4",
		"https://video.example.com/" + strings.Repeat("a", maxSameOriginVideoResultURLLength),
	} {
		_, err := ValidateSameOriginHTTPSVideoResultURL(candidate, "https://video.example.com")
		require.Error(t, err, candidate)
	}
}
