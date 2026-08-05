package model

import (
	"slices"

	"github.com/QuantumNous/new-api/dto"
)

type ModelArkVideoDurationProjection struct {
	Minimum         int   `json:"minimum"`
	Maximum         int   `json:"maximum"`
	Default         *int  `json:"default,omitempty"`
	Values          []int `json:"values,omitempty"`
	AllowsAutomatic bool  `json:"allows_automatic"`
	Required        bool  `json:"required"`
}

type ModelArkVideoMediaProjection struct {
	ContentTypes []string `json:"content_types"`
	MinImages    int      `json:"min_images"`
	MaxImages    int      `json:"max_images"`
	MaxVideos    int      `json:"max_videos"`
	MaxAudio     int      `json:"max_audio"`
	ImageRoles   []string `json:"image_roles,omitempty"`
	VideoRoles   []string `json:"video_roles,omitempty"`
	AudioRoles   []string `json:"audio_roles,omitempty"`
}

type ModelArkVideoConstraintProjection struct {
	RequiresText            bool `json:"requires_text"`
	SupportsGenerateAudio   bool `json:"supports_generate_audio"`
	SupportsDirectMedia     bool `json:"supports_direct_media"`
	SupportsLinkAssets      bool `json:"supports_link_assets"`
	SupportsMixedMediaPaths bool `json:"supports_mixed_media_paths"`
	ReferenceModesExclusive bool `json:"reference_modes_exclusive"`
	AudioRequiresReference  bool `json:"audio_requires_reference_image"`
	AudioRequiresVisual     bool `json:"audio_requires_visual_reference"`
}

type ModelArkVideoCapabilityProjection struct {
	PublicModel                 string                            `json:"public_model"`
	ContractID                  string                            `json:"contract_id"`
	Version                     string                            `json:"version"`
	ContentHash                 string                            `json:"content_hash"`
	RequestFields               []string                          `json:"request_fields"`
	RequiredFields              []string                          `json:"required_fields"`
	UnsupportedFields           []string                          `json:"unsupported_fields"`
	Resolutions                 []string                          `json:"resolutions"`
	DefaultResolution           string                            `json:"default_resolution,omitempty"`
	Ratios                      []string                          `json:"ratios"`
	DefaultRatio                string                            `json:"default_ratio,omitempty"`
	ResolutionRatioCombinations []VideoResolutionRatioCombination `json:"resolution_ratio_combinations,omitempty"`
	RequiresResolution          bool                              `json:"requires_resolution"`
	RequiresRatio               bool                              `json:"requires_ratio"`
	Duration                    ModelArkVideoDurationProjection   `json:"duration"`
	Media                       ModelArkVideoMediaProjection      `json:"media"`
	Constraints                 ModelArkVideoConstraintProjection `json:"constraints"`
	Lifecycle                   VideoSKULifecycleCapability       `json:"lifecycle"`
}

// modelArkOpenAPIModelIDs selects the already approved public documentation
// surface. It does not publish models or replace LinkModelPublication; every
// projected value is derived from the runtime capability registry.
var modelArkOpenAPIModelIDs = []string{
	VideoSKUDoubaoSeedance20260128,
	VideoSKUSeedance20Fast,
	VideoSKUSeedance20Oversea,
	VideoSKUSeedance20Standard,
	VideoSKUSeedanceBytePlus,
}

// RegisteredModelArkVideoCapabilityProjection returns every code-registered
// ModelArk video SKU, including candidates that are not currently published or
// fulfillable. Callers must expose availability separately and must not infer it
// from registry membership.
func RegisteredModelArkVideoCapabilityProjection() []ModelArkVideoCapabilityProjection {
	modelIDs := make([]string, 0, len(videoSKUCapabilities))
	for publicModel, capability := range videoSKUCapabilities {
		if capability.ContractID == string(dto.VideoContractModelArkV3) {
			modelIDs = append(modelIDs, publicModel)
		}
	}
	return modelArkVideoCapabilityProjection(modelIDs)
}

func PublicModelArkVideoCapabilityProjection() []ModelArkVideoCapabilityProjection {
	return modelArkVideoCapabilityProjection(modelArkOpenAPIModelIDs)
}

func modelArkVideoCapabilityProjection(modelIDs []string) []ModelArkVideoCapabilityProjection {
	projections := make([]ModelArkVideoCapabilityProjection, 0, len(modelIDs))
	for _, publicModel := range modelIDs {
		capability, ok := ResolveVideoSKUCapability(publicModel)
		if !ok {
			continue
		}
		resolutions := append([]string(nil), capability.Resolutions...)
		if capability.Resolution != "" {
			resolutions = []string{capability.Resolution}
		}
		contentTypes := []string{"text"}
		if capability.MaxImages > 0 {
			contentTypes = append(contentTypes, "image_url")
		}
		if capability.MaxVideos > 0 {
			contentTypes = append(contentTypes, "video_url")
		}
		if capability.MaxAudio > 0 {
			contentTypes = append(contentTypes, "audio_url")
		}
		var defaultDuration *int
		if capability.DefaultDuration > 0 {
			value := capability.DefaultDuration
			defaultDuration = &value
		}
		projections = append(projections, ModelArkVideoCapabilityProjection{
			PublicModel:                 capability.PublicModel,
			ContractID:                  capability.ContractID,
			Version:                     capability.Version,
			ContentHash:                 capability.ContentHash,
			RequestFields:               append([]string(nil), capability.RequestFields...),
			RequiredFields:              append([]string(nil), capability.RequiredFields...),
			UnsupportedFields:           append([]string(nil), capability.UnsupportedFields...),
			Resolutions:                 resolutions,
			DefaultResolution:           capability.DefaultResolution,
			Ratios:                      append([]string(nil), capability.Ratios...),
			DefaultRatio:                capability.DefaultRatio,
			ResolutionRatioCombinations: append([]VideoResolutionRatioCombination(nil), capability.ResolutionRatioCombinations...),
			RequiresResolution:          capability.RequiresResolution || capability.DefaultResolution == "",
			RequiresRatio:               capability.RequiresRatio || capability.DefaultRatio == "",
			Duration: ModelArkVideoDurationProjection{
				Minimum: capability.MinDuration, Maximum: capability.MaxDuration,
				Default: defaultDuration, Values: append([]int(nil), capability.DurationValues...),
				AllowsAutomatic: capability.AllowsAutomaticDuration,
				Required:        capability.RequiresDuration || capability.DefaultDuration <= 0,
			},
			Media: ModelArkVideoMediaProjection{
				ContentTypes: contentTypes, MinImages: capability.MinImages,
				MaxImages: capability.MaxImages, MaxVideos: capability.MaxVideos, MaxAudio: capability.MaxAudio,
				ImageRoles: append([]string(nil), capability.ImageRoles...),
				VideoRoles: append([]string(nil), capability.VideoRoles...),
				AudioRoles: append([]string(nil), capability.AudioRoles...),
			},
			Constraints: ModelArkVideoConstraintProjection{
				RequiresText: capability.RequiresText, SupportsGenerateAudio: capability.SupportsGenerateAudio,
				SupportsDirectMedia: capability.SupportsDirectMedia, SupportsLinkAssets: capability.SupportsLinkAssets,
				SupportsMixedMediaPaths: capability.SupportsMixedMediaPath,
				ReferenceModesExclusive: capability.ReferenceModesExclusive,
				AudioRequiresReference:  capability.AudioRequiresReference, AudioRequiresVisual: capability.AudioRequiresVisual,
			},
			Lifecycle: capability.Lifecycle,
		})
	}
	slices.SortFunc(projections, func(left, right ModelArkVideoCapabilityProjection) int {
		if left.PublicModel < right.PublicModel {
			return -1
		}
		if left.PublicModel > right.PublicModel {
			return 1
		}
		return 0
	})
	return projections
}
