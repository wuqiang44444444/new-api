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

	_, legacyRegistered := ChannelType2APIType(63)
	assert.False(t, legacyRegistered)
	assert.Equal(t, 63, constant.ChannelTypeDummy)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeDummy)
	assert.Empty(t, constant.ChannelBaseURLs[63])
}
