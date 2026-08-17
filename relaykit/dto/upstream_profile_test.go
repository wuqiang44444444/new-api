package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeicaiVideoProfileIsExplicitAndScoped(t *testing.T) {
	profile := VideoUpstreamProfileThirdPartyFeicaiVideos
	assert.True(t, profile.IsValid())
	assert.True(t, profile.IsThirdParty())
	assert.False(t, profile.IsOfficial())
	assert.NoError(t, ValidateVideoUpstreamProfile(profile))
}

func TestFunCloudVideoProfileIsExplicitAndScoped(t *testing.T) {
	video := VideoUpstreamProfileThirdPartyFunCloudSeedance
	assert.True(t, video.IsValid())
	assert.True(t, video.IsThirdParty())
	assert.False(t, video.IsOfficial())
	assert.NoError(t, ValidateVideoUpstreamProfile(video))
	assert.Error(t, ValidateVideoUpstreamProfile("third_party_funcloud_seedance_v2"))
}
