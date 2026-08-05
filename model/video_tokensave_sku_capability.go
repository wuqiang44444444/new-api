package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const VideoSKUCapabilityVersionTokenSaveV2 = "tokensave-media-task-v2"

func tokenSaveSeedanceVideoSKUCapability() VideoSKUCapability {
	capability := VideoSKUCapability{
		PublicModel:             VideoSKUDoubaoSeedance20260128,
		ContractID:              string(dto.VideoContractModelArkV3),
		Version:                 VideoSKUCapabilityVersionTokenSaveV2,
		Resolutions:             []string{"480p", "720p", "1080p"},
		MinDuration:             4,
		MaxDuration:             15,
		AllowsAutomaticDuration: true,
		RequiresDuration:        true,
		RequiresResolution:      true,
		RequiresRatio:           true,
		MaxPromptCharacters:     2500,
		Ratios:                  canonicalModelArkRatios(),
		MaxImages:               9,
		ImageRoles:              []string{"", "first_frame", "last_frame", "reference_image"},
		SupportsGenerateAudio:   true,
		SupportsDirectMedia:     true,
		SupportsLinkAssets:      true,
		ReferenceModesExclusive: true,
		RequiresText:            true,
		RequestFields:           []string{"model", "content", "duration", "resolution", "ratio", "generate_audio"},
		RequiredFields:          []string{"model", "content"},
		UnsupportedFields: []string{
			"end_user_subject",
			"callback_url",
			"service_tier",
			"watermark",
			"return_last_frame",
			"execution_expires_after",
			"draft",
			"tools",
			"safety_identifier",
			"priority",
			"frames",
			"seed",
			"camera_fixed",
		},
		RequiredChannelTypes: []int{constant.ChannelTypeDoubaoVideo},
		RequiredProfiles:     []string{VideoProfileThirdPartyRelay},
		Lifecycle:            VideoSKULifecycleCapability{SupportsContent: true},
	}
	capability.ContentHash = videoSKUCapabilityHash(capability)
	return capability
}
