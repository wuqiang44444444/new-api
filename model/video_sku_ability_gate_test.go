package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestVideoSKUAbilityPublishGateRejectsIncompatibleBinding(t *testing.T) {
	feicai := &Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Models: VideoSKUSeedance20Standard720P,
		Status: common.ChannelStatusEnabled,
	}
	feicai.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:    dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
		VideoUpstreamCreatePath: "/v1/videos", VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
		LinkImplementation: dto.LinkImplementationRef{ID: LinkImplementationFeicaiSeedanceJSON, Version: LinkImplementationVersionV1},
	})
	require.NoError(t, ValidateLinkSKUAbilityBindings(feicai))

	feicai.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
	})
	require.Error(t, ValidateLinkSKUAbilityBindings(feicai))

	bytePlus := &Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Models: VideoSKUSeedanceBytePlus,
		Status: common.ChannelStatusEnabled,
	}
	bytePlus.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
		AssetUpstreamProfile: dto.AssetUpstreamProfileOfficial,
		LinkImplementation:   dto.LinkImplementationRef{ID: LinkImplementationBytePlusSeedanceArk, Version: LinkImplementationVersionV1},
	})
	require.NoError(t, ValidateLinkSKUAbilityBindings(bytePlus))
	bytePlusCapability, ok := ResolveVideoSKUCapability(VideoSKUSeedanceBytePlus)
	require.True(t, ok)
	require.True(t, bytePlusCapability.SupportsProfile(""))
	require.True(t, bytePlusCapability.Lifecycle.SupportsCancelQueued)
	require.True(t, bytePlusCapability.Lifecycle.SupportsDelete)
	bytePlus.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyRelay,
	})
	require.Error(t, ValidateLinkSKUAbilityBindings(bytePlus))

	doubao := &Channel{
		Type:   constant.ChannelTypeDoubaoVideo,
		Models: VideoSKUDoubaoSeedance20260128,
		Status: common.ChannelStatusEnabled,
	}
	doubao.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile:    dto.VideoUpstreamProfileThirdPartyRelay,
		AssetUpstreamProfile:    dto.AssetUpstreamProfileRelay,
		VideoUpstreamCreatePath: "/v1/media/generations", VideoUpstreamQueryPathTemplate: "/v1/media/tasks/{task_id}",
		LinkImplementation: dto.LinkImplementationRef{ID: LinkImplementationTokenSaveSeedance, Version: LinkImplementationVersionV1},
	})
	require.NoError(t, ValidateLinkSKUAbilityBindings(doubao))
	doubaoCapability, ok := ResolveVideoSKUCapability(VideoSKUDoubaoSeedance20260128)
	require.True(t, ok)
	require.Equal(t, []string{"480p", "720p", "1080p"}, doubaoCapability.Resolutions)
	require.True(t, doubaoCapability.SupportsProfile(VideoProfileThirdPartyRelay))
	doubao.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyReverseProxy,
	})
	require.Error(t, ValidateLinkSKUAbilityBindings(doubao))

	kling := &Channel{
		Type:   constant.ChannelTypeKling,
		Models: VideoSKUKlingV1,
		Status: common.ChannelStatusEnabled,
	}
	kling.SetOtherSettings(dto.ChannelOtherSettings{LinkImplementation: dto.LinkImplementationRef{ID: LinkImplementationKlingVideos, Version: LinkImplementationVersionV1}})
	require.NoError(t, ValidateLinkSKUAbilityBindings(kling))
	kling.Type = constant.ChannelTypeDoubaoVideo
	require.Error(t, ValidateLinkSKUAbilityBindings(kling))
	kling.Type = constant.ChannelTypeKling
	kling.Models = "unregistered-kling-model"
	require.NoError(t, ValidateLinkSKUAbilityBindings(kling))

	overseaCapability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Oversea)
	require.True(t, ok)
	require.True(t, overseaCapability.SupportsProfile(VideoProfileThirdPartyRelay))
	require.True(t, overseaCapability.SupportsProfile(VideoProfileThirdPartyReverse))
	require.False(t, overseaCapability.Lifecycle.SupportsCancelQueued)
	require.False(t, overseaCapability.Lifecycle.SupportsDelete)
	moxingArk := &Channel{
		Type: constant.ChannelTypeDoubaoVideo, Models: VideoSKUSeedance20Oversea, Status: common.ChannelStatusEnabled,
	}
	moxingArk.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyReverseProxy})
	require.Error(t, ValidateLinkSKUAbilityBindings(moxingArk))
}

func TestVideoSKUImplementationEquivalenceIncludesLifecycleAndRequestLimits(t *testing.T) {
	public, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	implementation, ok := ResolveVideoSKUImplementationCapability(
		public.PublicModel,
		dto.LinkImplementationRef{ID: LinkImplementationFeicaiSeedanceJSON, Version: LinkImplementationVersionV1},
	)
	require.True(t, ok)
	require.True(t, VideoSKUCapabilitiesEquivalent(public, implementation))

	implementation.Lifecycle.SupportsDelete = true
	require.False(t, VideoSKUCapabilitiesEquivalent(public, implementation))

	implementation = public
	implementation.MaxImages--
	require.False(t, VideoSKUCapabilitiesEquivalent(public, implementation))

	original := videoSKUCapabilities[VideoSKUSeedance20Standard720P]
	changed := original
	changed.MaxImages--
	changed.ContentHash = videoSKUCapabilityHash(changed)
	videoSKUCapabilities[VideoSKUSeedance20Standard720P] = changed
	t.Cleanup(func() {
		videoSKUCapabilities[VideoSKUSeedance20Standard720P] = original
	})

	public, ok = ResolveVideoSKUCapability(VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	implementation, ok = ResolveVideoSKUImplementationCapability(
		public.PublicModel,
		dto.LinkImplementationRef{ID: LinkImplementationFeicaiSeedanceJSON, Version: LinkImplementationVersionV1},
	)
	require.True(t, ok)
	require.False(t, VideoSKUCapabilitiesEquivalent(public, implementation))
}
