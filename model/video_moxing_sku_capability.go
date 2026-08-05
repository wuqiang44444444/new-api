package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const VideoSKUCapabilityVersionMoxingV2 = "moxing-media-task-v2"

func moxingSeedanceVideoSKUCapability() VideoSKUCapability {
	capability := VideoSKUCapability{
		PublicModel:             VideoSKUSeedance20Oversea,
		ContractID:              string(dto.VideoContractModelArkV3),
		Version:                 VideoSKUCapabilityVersionMoxingV2,
		Resolutions:             []string{"480p", "720p"},
		MinDuration:             4,
		MaxDuration:             15,
		AllowsAutomaticDuration: true,
		RequiresDuration:        true,
		RequiresResolution:      true,
		RequiresRatio:           true,
		MaxPromptCharacters:     2500,
		Ratios:                  []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"},
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
