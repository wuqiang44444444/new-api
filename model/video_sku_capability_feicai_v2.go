package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const (
	VideoBillingModePerSecond  = "per-second"
	VideoBillingModePerRequest = "per-request"

	FeicaiProviderModelSeedance20Mini720P      = "seedance-2.0-vip-720p-mini-azhw-feicai"
	FeicaiProviderModelSeedance20SD2720P       = "seedance2.0-sd2-feicai"
	FeicaiProviderModelSeedance20Fast720P      = "seedance-2.0-vip-720p-fast-azhw-feicai"
	FeicaiProviderModelSeedance20Value720P     = "seedance-2.0-933-720p-azhw-feicai"
	FeicaiProviderModelSeedance20Standard720P  = "seedance-2.0-vip-720p-azhw-feicai"
	FeicaiProviderModelSeedance20Value1080P    = "seedance-2.0-933-1080p-azhw-feicai"
	FeicaiProviderModelSeedance20Standard1080P = "seedance-2.0-vip-1080p-azhw-feicai"
	FeicaiProviderModelSeedance20Value4K       = "seedance-2.0-933-4k-azhw-feicai"
	FeicaiProviderModelSeedance20Standard4K    = "seedance-2.0-vip-4k-azhw-feicai"
	FeicaiProviderModelSeedance20ProPI720P     = "seedance-933-pro-pi-feicai"
)

type feicaiVideoSKUDefinition struct {
	PublicModel     string
	Resolution      string
	MinDuration     int
	MaxDuration     int
	DefaultDuration int
	MinImages       int
	MaxAudio        int
	MaxVideos       int
	BillingMode     string
	Ratios          []string
}

func feicaiVideoSKUCapabilities() map[string]VideoSKUCapability {
	definitions := []feicaiVideoSKUDefinition{
		{PublicModel: VideoSKUSeedance20Mini720P, Resolution: "720p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20SD2720P, Resolution: "720p", MinDuration: 11, MaxDuration: 15, MinImages: 1, BillingMode: VideoBillingModePerSecond},
		{PublicModel: VideoSKUSeedance20Fast720P, Resolution: "720p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20Value720P, Resolution: "720p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20Standard720P, Resolution: "720p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20Value1080P, Resolution: "1080p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond},
		{PublicModel: VideoSKUSeedance20Standard1080P, Resolution: "1080p", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20Value4K, Resolution: "4k", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond},
		{PublicModel: VideoSKUSeedance20Standard4K, Resolution: "4k", MinDuration: 4, MaxDuration: 15, MaxAudio: 3, BillingMode: VideoBillingModePerSecond, Ratios: []string{"16:9"}},
		{PublicModel: VideoSKUSeedance20ProPI720P, Resolution: "720p", MinDuration: 15, MaxDuration: 15, DefaultDuration: 15, MaxAudio: 3, MaxVideos: 3, BillingMode: VideoBillingModePerRequest},
	}
	result := make(map[string]VideoSKUCapability, len(definitions))
	for _, definition := range definitions {
		version := VideoSKUCapabilityVersionFeicaiV2
		if definition.PublicModel == VideoSKUSeedance20Fast720P || definition.PublicModel == VideoSKUSeedance20Standard4K {
			version = VideoSKUCapabilityVersionFeicaiV2R2
		} else if definition.PublicModel == VideoSKUSeedance20Value720P {
			version = VideoSKUCapabilityVersionFeicaiV2R3
		}
		capability := VideoSKUCapability{
			PublicModel:             definition.PublicModel,
			ContractID:              string(dto.VideoContractModelArkV3),
			Version:                 version,
			Resolution:              definition.Resolution,
			Resolutions:             []string{definition.Resolution},
			MinDuration:             definition.MinDuration,
			MaxDuration:             definition.MaxDuration,
			DefaultDuration:         definition.DefaultDuration,
			BillingMode:             definition.BillingMode,
			RequiresDuration:        definition.DefaultDuration <= 0,
			RequiresResolution:      true,
			RequiresRatio:           true,
			Ratios:                  append([]string(nil), definition.Ratios...),
			MinImages:               definition.MinImages,
			MaxImages:               9,
			MaxVideos:               definition.MaxVideos,
			MaxAudio:                definition.MaxAudio,
			ImageRoles:              []string{"reference_image"},
			AudioRoles:              []string{"reference_audio"},
			SupportsDirectMedia:     true,
			SupportsLinkAssets:      true,
			SupportsMixedMediaPath:  false,
			ReferenceModesExclusive: true,
			RequiresText:            true,
			AudioRequiresVisual:     definition.MaxAudio > 0,
			RequestFields:           []string{"model", "content", "duration", "resolution", "ratio"},
			RequiredFields:          []string{"model", "content"},
			UnsupportedFields: []string{
				"end_user_subject",
				"callback_url",
				"service_tier",
				"generate_audio",
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
			RequiredProfiles:     []string{VideoProfileJSONMediaArrays},
			Lifecycle:            VideoSKULifecycleCapability{SupportsContent: true},
		}
		if definition.MaxVideos > 0 {
			capability.VideoRoles = []string{"reference_video"}
		}
		capability.ContentHash = videoSKUCapabilityHash(capability)
		result[capability.PublicModel] = capability
	}
	return result
}
