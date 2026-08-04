package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVideoUpstreamProfileIsOfficial(t *testing.T) {
	// 空字符串与 official 运行语义一致，旧渠道没有该字段时视为官方。
	assert.True(t, VideoUpstreamProfile("").IsOfficial())
	assert.True(t, VideoUpstreamProfileOfficial.IsOfficial())
	assert.False(t, VideoUpstreamProfileThirdPartyReverseProxy.IsOfficial())
	assert.False(t, VideoUpstreamProfileThirdPartyRelay.IsOfficial())
	assert.False(t, VideoUpstreamProfileThirdPartyJSONVideoMediaArrays.IsOfficial())
}

func TestVideoUpstreamProfileIsThirdParty(t *testing.T) {
	assert.False(t, VideoUpstreamProfile("").IsThirdParty())
	assert.False(t, VideoUpstreamProfileOfficial.IsThirdParty())
	assert.True(t, VideoUpstreamProfileThirdPartyRelay.IsThirdParty())
	assert.True(t, VideoUpstreamProfileThirdPartyReverseProxy.IsThirdParty())
	assert.True(t, VideoUpstreamProfileThirdPartyJSONVideoMediaArrays.IsThirdParty())
}

func TestVideoUpstreamProfileIsValid(t *testing.T) {
	for _, p := range []VideoUpstreamProfile{
		VideoUpstreamProfileOfficial,
		VideoUpstreamProfileThirdPartyRelay,
		VideoUpstreamProfileThirdPartyReverseProxy,
		VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	} {
		assert.Truef(t, p.IsValid(), "expected %q to be a registered profile", p)
	}
	// 空字符串不是注册 ID（仅在校验时被当作 official 接受）；未知值不命中注册集合。
	assert.False(t, VideoUpstreamProfile("").IsValid())
	assert.False(t, VideoUpstreamProfile("third_party_unknown").IsValid())
	assert.False(t, VideoUpstreamProfile("Official").IsValid()) // 大小写敏感
}

func TestValidateVideoUpstreamProfile(t *testing.T) {
	// 空与已知 ID 通过（保存时允许）。
	for _, p := range []VideoUpstreamProfile{
		"",
		VideoUpstreamProfileOfficial,
		VideoUpstreamProfileThirdPartyRelay,
		VideoUpstreamProfileThirdPartyReverseProxy,
		VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	} {
		assert.NoErrorf(t, ValidateVideoUpstreamProfile(p), "expected %q to pass validation", p)
	}
	// 未知 ID 在渠道保存时被拒绝（方案 §5）。
	for _, p := range []VideoUpstreamProfile{"third_party_unknown", "official2", "garbage"} {
		assert.Errorf(t, ValidateVideoUpstreamProfile(p), "expected unknown profile %q to be rejected", p)
	}
}
