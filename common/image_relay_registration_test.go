package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageRelayUsesOneChannelAndImageAPIRegistration(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeAsyncImage)
	require.True(t, ok)
	assert.Equal(t, constant.APITypeAsyncImage, apiType)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeImageGeneration}, GetEndpointTypesByChannelType(constant.ChannelTypeAsyncImage, "custom-model"))

	// Upstream rc.31 owns channel type 61 (Task Plugin); the async image type
	// was renumbered to 63 and the retired Moxing slot moved to 65.
	_, legacyRegistered := ChannelType2APIType(61)
	assert.False(t, legacyRegistered)
	assert.Equal(t, 64, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeAsyncImage)
	assert.Equal(t, "https://mm-internal-cn.leonecloud.com", constant.ChannelBaseURLs[constant.ChannelTypeAsyncImage])
}
