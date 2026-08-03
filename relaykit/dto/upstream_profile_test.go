package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJSONVideoOmniReferenceProfileClassification(t *testing.T) {
	profile := VideoUpstreamProfileThirdPartyJSONVideoOmniReference
	assert.True(t, profile.IsValid())
	assert.True(t, profile.IsThirdParty())
	assert.False(t, profile.IsOfficial())
	assert.NoError(t, ValidateVideoUpstreamProfile(profile))
	assert.Error(t, ValidateVideoUpstreamProfile("third_party_json_video_unknown"))
}

func TestFunCloudVideoProfileIsExplicitAndScoped(t *testing.T) {
	video := VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
	assert.True(t, video.IsValid())
	assert.True(t, video.IsThirdParty())
	assert.False(t, video.IsOfficial())
	assert.NoError(t, ValidateVideoUpstreamProfile(video))
}
