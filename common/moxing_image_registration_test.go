package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxingImageChannelRegistrationUsesDedicatedImageAPI(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeMoxingImage)
	require.True(t, ok)
	assert.Equal(t, constant.APITypeMoxingImage, apiType)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeImageGeneration}, GetEndpointTypesByChannelType(constant.ChannelTypeMoxingImage, "custom-model"))
}

func TestMoxingImageChannelKeepsSentinelsAndBaseURLIndexesAligned(t *testing.T) {
	assert.Equal(t, 63, constant.ChannelTypeMoxingImage)
	assert.Equal(t, constant.ChannelTypeMoxingImage, constant.ChannelTypeDummy)
	assert.Greater(t, constant.APITypeDummy, constant.APITypeMoxingImage)
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeMoxingImage)
	assert.Equal(t, "https://www.moxing.pro", constant.ChannelBaseURLs[constant.ChannelTypeMoxingImage])
}
