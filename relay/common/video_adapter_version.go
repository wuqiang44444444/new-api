package common

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	VideoAdapterRevisionV1 = "v1"
	VideoAdapterRevisionV2 = "v2"
)

// VideoSouthboundAdapterVersion is the compatibility name for the frozen channel
// adapter protocol selected when a video task is created. Profile identifies the
// request/query shape; Revision identifies versioned result and credential semantics.
type VideoSouthboundAdapterVersion struct {
	ChannelType int
	Profile     dto.VideoUpstreamProfile
	Revision    string
}

type videoAdapterRevisionRule struct {
	Revision   string
	AllowEmpty bool
}

var videoAdapterRevisionRules = map[struct {
	ChannelType int
	Profile     dto.VideoUpstreamProfile
}]videoAdapterRevisionRule{
	{ChannelType: constant.ChannelTypeDoubaoVideo, Profile: dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays}: {Revision: VideoAdapterRevisionV1, AllowEmpty: false},
	{ChannelType: constant.ChannelTypeDoubaoVideo, Profile: dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2}:   {Revision: VideoAdapterRevisionV2, AllowEmpty: false},
}

func (version VideoSouthboundAdapterVersion) String() string {
	return fmt.Sprintf("%d:%s:%s", version.ChannelType, version.Profile, version.Revision)
}

func (version VideoSouthboundAdapterVersion) IsJSONVideoMediaArraysV1() bool {
	return version.ChannelType == constant.ChannelTypeDoubaoVideo &&
		version.Profile == dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays &&
		version.Revision == VideoAdapterRevisionV1
}

func (version VideoSouthboundAdapterVersion) IsFunCloudSeedanceV2() bool {
	return version.ChannelType == constant.ChannelTypeDoubaoVideo &&
		version.Profile == dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2 &&
		version.Revision == VideoAdapterRevisionV2
}

// CurrentVideoSouthboundAdapterVersion is the single authority for new channel
// adapter protocol snapshots. Existing tasks must use
// ResolveVideoSouthboundAdapterVersion and never reinterpret their frozen contract.
func CurrentVideoSouthboundAdapterVersion(channelType int, profile dto.VideoUpstreamProfile) string {
	profile = normalizedVideoUpstreamProfile(profile)
	rule := videoAdapterRevisionRuleFor(channelType, profile)
	return VideoSouthboundAdapterVersion{
		ChannelType: channelType,
		Profile:     profile,
		Revision:    rule.Revision,
	}.String()
}

// ResolveVideoSouthboundAdapterVersion validates that a frozen channel adapter
// protocol version belongs to the selected channel/profile. Profiles with
// credential-sensitive result semantics require an explicit frozen version;
// unknown and mismatched versions fail closed.
func ResolveVideoSouthboundAdapterVersion(
	channelType int,
	profile dto.VideoUpstreamProfile,
	frozen string,
) (VideoSouthboundAdapterVersion, error) {
	profile = normalizedVideoUpstreamProfile(profile)
	if channelType <= 0 {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter channel type is invalid")
	}
	if !profile.IsValid() {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter profile is invalid")
	}

	frozen = strings.TrimSpace(frozen)
	if frozen == "" {
		rule := videoAdapterRevisionRuleFor(channelType, profile)
		if !rule.AllowEmpty {
			return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter version is required")
		}
		return VideoSouthboundAdapterVersion{
			ChannelType: channelType,
			Profile:     profile,
			Revision:    rule.Revision,
		}, nil
	}

	parts := strings.Split(frozen, ":")
	if len(parts) != 3 {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter version is malformed")
	}
	frozenChannelType, err := strconv.Atoi(parts[0])
	if err != nil || frozenChannelType != channelType {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter channel type mismatch")
	}
	frozenProfile := dto.VideoUpstreamProfile(parts[1])
	if frozenProfile != profile {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter profile mismatch")
	}
	revision := parts[2]
	if revision != videoAdapterRevisionRuleFor(channelType, profile).Revision {
		return VideoSouthboundAdapterVersion{}, fmt.Errorf("video adapter revision is unsupported")
	}
	return VideoSouthboundAdapterVersion{
		ChannelType: frozenChannelType,
		Profile:     frozenProfile,
		Revision:    revision,
	}, nil
}

func videoAdapterRevisionRuleFor(channelType int, profile dto.VideoUpstreamProfile) videoAdapterRevisionRule {
	if rule, ok := videoAdapterRevisionRules[struct {
		ChannelType int
		Profile     dto.VideoUpstreamProfile
	}{ChannelType: channelType, Profile: profile}]; ok {
		return rule
	}
	return videoAdapterRevisionRule{Revision: VideoAdapterRevisionV1, AllowEmpty: true}
}

func normalizedVideoUpstreamProfile(profile dto.VideoUpstreamProfile) dto.VideoUpstreamProfile {
	if profile == "" {
		return dto.VideoUpstreamProfileOfficial
	}
	return profile
}
