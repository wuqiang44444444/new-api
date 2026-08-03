package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateHTTPSVideoResultURLAllowsCrossOriginHTTPSOnly(t *testing.T) {
	value, err := ValidateHTTPSVideoResultURL("https://cdn.example/video.mp4?signature=1")
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.example/video.mp4?signature=1", value)
	_, err = ValidateHTTPSVideoResultURL("http://cdn.example/video.mp4")
	assert.Error(t, err)
	_, err = ValidateHTTPSVideoResultURL("https://user:pass@cdn.example/video.mp4")
	assert.Error(t, err)
}
