package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

func funCloudVideoSKUCapabilities() map[string]VideoSKUCapability {
	result := make(map[string]VideoSKUCapability, 2)
	for publicModel, resolutions := range map[string][]string{
		VideoSKUSeedance20Standard: {"480p", "720p", "1080p"},
		VideoSKUSeedance20Fast:     {"480p", "720p"},
	} {
		capability := VideoSKUCapability{
			PublicModel:            publicModel,
			ContractID:             string(dto.VideoContractModelArkV3),
			Version:                VideoSKUCapabilityVersionV1,
			Resolutions:            resolutions,
			MinDuration:            4,
			MaxDuration:            15,
			DefaultDuration:        5,
			Ratios:                 []string{"16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive"},
			MaxImages:              3,
			MaxVideos:              1,
			MaxAudio:               1,
			ImageRoles:             []string{"reference_image", "first_frame", "last_frame"},
			VideoRoles:             []string{"reference_video"},
			AudioRoles:             []string{"reference_audio"},
			SupportsGenerateAudio:  true,
			SupportsDirectMedia:    true,
			SupportsLinkAssets:     true,
			SupportsMixedMediaPath: false,
			AudioRequiresVisual:    true,
			RequiresText:           true,
			RequestFields: []string{
				"model", "end_user_subject", "content", "duration", "resolution", "ratio",
				"generate_audio", "watermark", "seed", "camera_fixed",
			},
			RequiredFields: []string{"model", "content"},
			UnsupportedFields: []string{
				"callback_url", "service_tier", "return_last_frame", "execution_expires_after",
				"draft", "tools", "safety_identifier", "priority", "frames",
			},
			RequiredChannelTypes: []int{constant.ChannelTypeDoubaoVideo},
			RequiredProfiles:     []string{VideoProfileFunCloudSeedanceV2},
			Lifecycle:            VideoSKULifecycleCapability{SupportsContent: true},
		}
		capability.ContentHash = videoSKUCapabilityHash(capability)
		result[publicModel] = capability
	}
	return result
}
