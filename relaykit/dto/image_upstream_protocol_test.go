package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageUpstreamProtocolValidation(t *testing.T) {
	for _, protocol := range []ImageUpstreamProtocol{
		ImageUpstreamProtocolFunCloudAIGCV2,
		ImageUpstreamProtocolMoxingImagesV1,
	} {
		require.NoError(t, ValidateImageUpstreamProtocol(protocol))
		assert.True(t, protocol.IsValid())
	}

	err := ValidateImageUpstreamProtocol("inferred_from_model")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported image upstream protocol")
}
