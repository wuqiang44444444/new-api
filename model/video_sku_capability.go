package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const (
	VideoSKUCapabilityVersionAZHWV1 = "azhw-media-arrays-v1"
	VideoSKUCapabilityVersionV1     = "public-video-contract-v1"
	VideoProfileJSONMediaArrays     = "third_party_json_video_media_arrays"
	VideoProfileOfficial            = "official"
	VideoProfileThirdPartyRelay     = "third_party_relay"
	VideoProfileThirdPartyReverse   = "third_party_reverse_proxy"
	VideoProfileFunCloudSeedanceV2  = "third_party_funcloud_seedance_v2"

	VideoSKUSeedanceBytePlus        = "seedance-byteplus"
	VideoSKUSeedance20Oversea       = "seedance-2-0-oversea"
	VideoSKUDoubaoSeedance20260128  = "doubao-seedance-2-0-260128"
	VideoSKUSeedance20Standard720P  = "seedance-2.0-standard-720p"
	VideoSKUSeedance20Standard1080P = "seedance-2.0-standard-1080p"
	VideoSKUSeedance20Value720P     = "seedance-2.0-value-720p"
	VideoSKUSeedance20Value1080P    = "seedance-2.0-value-1080p"
	VideoSKUSeedance20Value4K       = "seedance-2.0-value-4k"
	VideoSKUSeedance20Standard      = "seedance-2.0-standard"
	VideoSKUSeedance20Fast          = "seedance-2.0-fast"
	VideoSKUKlingV1                 = "kling-v1"
	VideoSKUKlingV16                = "kling-v1-6"
	VideoSKUKlingV2Master           = "kling-v2-master"
	VideoSKUJimengVGFMT2VL20        = "jimeng_vgfm_t2v_l20"
)

type VideoSKULifecycleCapability struct {
	SupportsContent      bool `json:"supports_content"`
	SupportsLastFrame    bool `json:"supports_last_frame"`
	SupportsCancelQueued bool `json:"supports_cancel_queued"`
	SupportsDelete       bool `json:"supports_delete"`
}

type VideoSKUCapability struct {
	PublicModel             string                      `json:"public_model"`
	ContractID              string                      `json:"contract_id"`
	Version                 string                      `json:"version"`
	ContentHash             string                      `json:"content_hash"`
	Resolution              string                      `json:"resolution"`
	Resolutions             []string                    `json:"resolutions,omitempty"`
	MinDuration             int                         `json:"min_duration"`
	MaxDuration             int                         `json:"max_duration"`
	DefaultDuration         int                         `json:"default_duration"`
	AllowsAutomaticDuration bool                        `json:"allows_automatic_duration"`
	DurationValues          []int                       `json:"duration_values,omitempty"`
	Ratios                  []string                    `json:"ratios"`
	Modes                   []string                    `json:"modes,omitempty"`
	HasCFGScaleRange        bool                        `json:"has_cfg_scale_range,omitempty"`
	MinCFGScale             float64                     `json:"min_cfg_scale,omitempty"`
	MaxCFGScale             float64                     `json:"max_cfg_scale,omitempty"`
	MaxImages               int                         `json:"max_images"`
	MaxVideos               int                         `json:"max_videos"`
	MaxAudio                int                         `json:"max_audio"`
	ImageRoles              []string                    `json:"image_roles,omitempty"`
	VideoRoles              []string                    `json:"video_roles,omitempty"`
	AudioRoles              []string                    `json:"audio_roles,omitempty"`
	SupportsGenerateAudio   bool                        `json:"supports_generate_audio"`
	SupportsDirectMedia     bool                        `json:"supports_direct_media"`
	SupportsLinkAssets      bool                        `json:"supports_link_assets"`
	SupportsMixedMediaPath  bool                        `json:"supports_mixed_media_paths"`
	ReferenceModesExclusive bool                        `json:"reference_modes_exclusive"`
	RequiresText            bool                        `json:"requires_text"`
	AudioRequiresReference  bool                        `json:"audio_requires_reference_image"`
	AudioRequiresVisual     bool                        `json:"audio_requires_visual_reference"`
	RequestFields           []string                    `json:"request_fields,omitempty"`
	RequiredFields          []string                    `json:"required_fields,omitempty"`
	UnsupportedFields       []string                    `json:"unsupported_fields,omitempty"`
	RequiredChannelTypes    []int                       `json:"required_channel_types"`
	RequiredProfiles        []string                    `json:"required_profiles"`
	Lifecycle               VideoSKULifecycleCapability `json:"lifecycle"`
}

// videoSKUCapabilities is the explicit code registry for published video Link
// contracts. A model belongs to this Link surface only when a code change adds
// it here with a contract ID and version; Ability and channel configuration do
// not infer or create Link-contract membership.
var videoSKUCapabilities = buildVideoSKUCapabilities()

func buildVideoSKUCapabilities() map[string]VideoSKUCapability {
	resolutions := map[string]string{
		VideoSKUSeedance20Standard720P: "720p",
		VideoSKUSeedance20Value720P:    "720p",
	}
	result := make(map[string]VideoSKUCapability, len(resolutions))
	for publicModel, resolution := range resolutions {
		capability := VideoSKUCapability{
			PublicModel:             publicModel,
			ContractID:              string(dto.VideoContractModelArkV3),
			Version:                 VideoSKUCapabilityVersionAZHWV1,
			Resolution:              resolution,
			Resolutions:             []string{resolution},
			MinDuration:             4,
			MaxDuration:             15,
			DefaultDuration:         4,
			Ratios:                  []string{"16:9", "9:16"},
			MaxImages:               9,
			MaxAudio:                3,
			ImageRoles:              []string{"reference_image"},
			AudioRoles:              []string{"reference_audio"},
			SupportsDirectMedia:     true,
			SupportsLinkAssets:      true,
			SupportsMixedMediaPath:  false,
			ReferenceModesExclusive: true,
			RequiresText:            true,
			AudioRequiresReference:  true,
			RequestFields: []string{
				"model", "content", "duration", "resolution", "ratio", "generate_audio",
			},
			RequiredFields: []string{"model", "content"},
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
			RequiredProfiles:     []string{VideoProfileJSONMediaArrays},
			Lifecycle:            VideoSKULifecycleCapability{SupportsContent: true},
		}
		capability.ContentHash = videoSKUCapabilityHash(capability)
		result[publicModel] = capability
	}
	for publicModel, profiles := range map[string][]string{
		VideoSKUSeedanceBytePlus:       {VideoProfileOfficial},
		VideoSKUSeedance20Oversea:      {VideoProfileThirdPartyRelay, VideoProfileThirdPartyReverse},
		VideoSKUDoubaoSeedance20260128: {VideoProfileThirdPartyRelay},
	} {
		lifecycle := VideoSKULifecycleCapability{
			SupportsContent:   true,
			SupportsLastFrame: true,
		}
		if slices.Contains(profiles, VideoProfileOfficial) {
			lifecycle.SupportsCancelQueued = true
			lifecycle.SupportsDelete = true
		}
		capability := VideoSKUCapability{
			PublicModel:             publicModel,
			ContractID:              string(dto.VideoContractModelArkV3),
			Version:                 VideoSKUCapabilityVersionV1,
			Resolutions:             []string{"480p", "720p", "1080p", "4k"},
			MinDuration:             4,
			MaxDuration:             15,
			DefaultDuration:         5,
			AllowsAutomaticDuration: true,
			Ratios:                  []string{"16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive"},
			MaxImages:               9,
			MaxVideos:               3,
			MaxAudio:                3,
			SupportsGenerateAudio:   true,
			SupportsDirectMedia:     true,
			SupportsLinkAssets:      true,
			SupportsMixedMediaPath:  false,
			AudioRequiresVisual:     true,
			RequestFields: []string{
				"model", "end_user_subject", "content", "duration", "resolution", "ratio", "service_tier",
				"generate_audio", "watermark", "return_last_frame", "execution_expires_after",
				"draft", "tools", "safety_identifier", "priority", "seed", "camera_fixed",
			},
			RequiredFields:       []string{"model", "content"},
			UnsupportedFields:    []string{"callback_url", "frames"},
			RequiredChannelTypes: []int{constant.ChannelTypeDoubaoVideo},
			RequiredProfiles:     append([]string(nil), profiles...),
			Lifecycle:            lifecycle,
		}
		if !slices.Contains(profiles, VideoProfileOfficial) {
			capability.MaxVideos = 0
			capability.MaxAudio = 0
			capability.ReferenceModesExclusive = true
			capability.UnsupportedFields = append(capability.UnsupportedFields,
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
			)
		}
		if publicModel == VideoSKUDoubaoSeedance20260128 {
			capability.Resolutions = []string{"480p", "720p", "1080p"}
		}
		capability.ContentHash = videoSKUCapabilityHash(capability)
		result[publicModel] = capability
	}
	for publicModel, capability := range funCloudVideoSKUCapabilities() {
		result[publicModel] = capability
	}
	for _, publicModel := range []string{VideoSKUKlingV1, VideoSKUKlingV16, VideoSKUKlingV2Master} {
		capability := VideoSKUCapability{
			PublicModel:         publicModel,
			ContractID:          string(dto.VideoContractKlingV1),
			Version:             VideoSKUCapabilityVersionV1,
			DurationValues:      []int{5, 10},
			Ratios:              []string{"16:9", "9:16", "1:1"},
			Modes:               []string{"std", "pro"},
			HasCFGScaleRange:    true,
			MinCFGScale:         0,
			MaxCFGScale:         1,
			MaxImages:           2,
			SupportsDirectMedia: true,
			SupportsLinkAssets:  true,
			RequiresText:        true,
			RequestFields: []string{
				"model_name", "prompt", "image", "image_tail", "negative_prompt", "mode",
				"duration", "aspect_ratio", "cfg_scale", "static_mask", "dynamic_masks",
				"camera_control", "callback_url", "external_task_id",
			},
			RequiredFields:       []string{"model_name", "prompt"},
			RequiredChannelTypes: []int{constant.ChannelTypeKling},
		}
		capability.ContentHash = videoSKUCapabilityHash(capability)
		result[publicModel] = capability
	}
	jimeng := VideoSKUCapability{
		PublicModel:         VideoSKUJimengVGFMT2VL20,
		ContractID:          string(dto.VideoContractJimeng),
		Version:             VideoSKUCapabilityVersionV1,
		SupportsDirectMedia: true,
		SupportsLinkAssets:  true,
		RequestFields: []string{
			"req_key", "binary_data_base64", "image_urls", "prompt", "seed", "aspect_ratio", "frames",
		},
		RequiredFields:       []string{"req_key"},
		RequiredChannelTypes: []int{constant.ChannelTypeJimeng},
	}
	jimeng.ContentHash = videoSKUCapabilityHash(jimeng)
	result[jimeng.PublicModel] = jimeng
	return result
}

func videoSKUCapabilityHash(capability VideoSKUCapability) string {
	capability.ContentHash = ""
	payload, err := common.Marshal(capability)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func ResolveVideoSKUCapability(publicModel string) (VideoSKUCapability, bool) {
	capability, ok := videoSKUCapabilities[strings.TrimSpace(publicModel)]
	if !ok {
		return VideoSKUCapability{}, false
	}
	return cloneVideoSKUCapability(capability), true
}

func cloneVideoSKUCapability(capability VideoSKUCapability) VideoSKUCapability {
	capability.Ratios = append([]string(nil), capability.Ratios...)
	capability.Resolutions = append([]string(nil), capability.Resolutions...)
	capability.DurationValues = append([]int(nil), capability.DurationValues...)
	capability.Modes = append([]string(nil), capability.Modes...)
	capability.ImageRoles = append([]string(nil), capability.ImageRoles...)
	capability.VideoRoles = append([]string(nil), capability.VideoRoles...)
	capability.AudioRoles = append([]string(nil), capability.AudioRoles...)
	capability.RequestFields = append([]string(nil), capability.RequestFields...)
	capability.RequiredFields = append([]string(nil), capability.RequiredFields...)
	capability.UnsupportedFields = append([]string(nil), capability.UnsupportedFields...)
	capability.RequiredChannelTypes = append([]int(nil), capability.RequiredChannelTypes...)
	capability.RequiredProfiles = append([]string(nil), capability.RequiredProfiles...)
	return capability
}

func (capability VideoSKUCapability) SupportsProfile(profile string) bool {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = VideoProfileOfficial
	}
	return slices.Contains(capability.RequiredProfiles, profile)
}

func (capability VideoSKUCapability) ValidateModelArkRequest(request *dto.ModelArkVideoCreateRequest) error {
	if request == nil || request.Model != capability.PublicModel {
		return fmt.Errorf("video SKU capability does not match request model")
	}
	if capability.ContractID != string(dto.VideoContractModelArkV3) {
		return fmt.Errorf("video SKU does not use the ModelArk contract")
	}
	for _, field := range capability.UnsupportedFields {
		present := false
		switch field {
		case "end_user_subject":
			present = request.EndUserSubject != nil
		case "callback_url":
			present = request.CallbackURL != nil
		case "service_tier":
			present = request.ServiceTier != nil
		case "watermark":
			present = request.Watermark != nil
		case "return_last_frame":
			present = request.ReturnLastFrame != nil
		case "execution_expires_after":
			present = request.ExecutionExpiresAfter != nil
		case "draft":
			present = request.Draft != nil
		case "tools":
			present = len(request.Tools) > 0
		case "safety_identifier":
			present = request.SafetyIdentifier != nil
		case "priority":
			present = request.Priority != nil
		case "frames":
			present = request.Frames != nil
		case "seed":
			present = request.Seed != nil
		case "camera_fixed":
			present = request.CameraFixed != nil
		default:
			return fmt.Errorf("video SKU capability contains unsupported field rule %q", field)
		}
		if present {
			return fmt.Errorf("%s is not supported by this model", field)
		}
	}
	if err := validateModelArkContractScalars(request); err != nil {
		return err
	}
	if request.Duration != nil {
		if *request.Duration == -1 && capability.AllowsAutomaticDuration {
			// The public contract explicitly permits provider-selected duration.
		} else if *request.Duration < capability.MinDuration || *request.Duration > capability.MaxDuration {
			return fmt.Errorf("duration must be between %d and %d", capability.MinDuration, capability.MaxDuration)
		}
	}
	if request.Resolution != nil {
		resolution := strings.TrimSpace(*request.Resolution)
		if capability.Resolution != "" && resolution != capability.Resolution {
			return fmt.Errorf("resolution must be %s for this model", capability.Resolution)
		}
		if capability.Resolution == "" && len(capability.Resolutions) > 0 &&
			!slices.Contains(capability.Resolutions, resolution) {
			return fmt.Errorf("resolution is not supported by this model")
		}
	}
	if request.Ratio != nil && !slices.Contains(capability.Ratios, strings.TrimSpace(*request.Ratio)) {
		return fmt.Errorf("ratio is not supported by this model")
	}
	if request.GenerateAudio != nil && *request.GenerateAudio && !capability.SupportsGenerateAudio {
		return fmt.Errorf("generate_audio is not supported by this model")
	}
	images, audio, video, text := 0, 0, 0, 0
	hasAsset, hasDirect := false, false
	firstFrame, lastFrame, referenceImage := 0, 0, 0
	for _, item := range request.Content {
		var mediaURL string
		switch item.Type {
		case "text":
			if strings.TrimSpace(videoString(item.Text)) != "" {
				text++
			}
		case "image_url":
			images++
			if item.ImageURL != nil {
				mediaURL = item.ImageURL.URL
			}
			role := strings.TrimSpace(videoString(item.Role))
			if len(capability.ImageRoles) > 0 && !slices.Contains(capability.ImageRoles, role) {
				return fmt.Errorf("image role %q is not supported by this model", role)
			}
			switch role {
			case "first_frame":
				firstFrame++
			case "last_frame":
				lastFrame++
			case "reference_image":
				referenceImage++
			}
		case "audio_url":
			audio++
			if item.AudioURL != nil {
				mediaURL = item.AudioURL.URL
			}
			role := strings.TrimSpace(videoString(item.Role))
			if len(capability.AudioRoles) > 0 && !slices.Contains(capability.AudioRoles, role) {
				return fmt.Errorf("audio role %q is not supported by this model", role)
			}
		case "video_url":
			video++
			if item.VideoURL != nil {
				mediaURL = item.VideoURL.URL
			}
			role := strings.TrimSpace(videoString(item.Role))
			if len(capability.VideoRoles) > 0 && !slices.Contains(capability.VideoRoles, role) {
				return fmt.Errorf("video role %q is not supported by this model", role)
			}
		}
		if strings.HasPrefix(strings.TrimSpace(mediaURL), "asset://") {
			hasAsset = true
		} else if mediaURL != "" {
			hasDirect = true
		}
	}
	switch {
	case capability.RequiresText && text == 0:
		return fmt.Errorf("at least one non-empty text item is required")
	case video > capability.MaxVideos:
		return fmt.Errorf("video content is not supported by this model")
	case images > capability.MaxImages:
		return fmt.Errorf("image content exceeds the maximum of %d", capability.MaxImages)
	case audio > capability.MaxAudio:
		return fmt.Errorf("audio content exceeds the maximum of %d", capability.MaxAudio)
	case hasAsset && !capability.SupportsLinkAssets:
		return fmt.Errorf("asset URLs are not supported by this model")
	case hasAsset && hasDirect && !capability.SupportsMixedMediaPath:
		return fmt.Errorf("asset and direct media URLs cannot be mixed")
	case lastFrame > 0 && firstFrame == 0:
		return fmt.Errorf("last_frame requires first_frame")
	case capability.ReferenceModesExclusive && firstFrame > 0 && referenceImage > 0:
		return fmt.Errorf("first_frame and reference_image cannot be combined")
	case capability.ReferenceModesExclusive && lastFrame > 0 && referenceImage > 0:
		return fmt.Errorf("last_frame and reference_image cannot be combined")
	case capability.ReferenceModesExclusive && lastFrame > 0 && audio > 0:
		return fmt.Errorf("both-frames mode does not support audio")
	case capability.AudioRequiresReference && audio > 0 && referenceImage == 0:
		return fmt.Errorf("audio requires reference_image content")
	case capability.AudioRequiresVisual && audio > 0 && images == 0 && video == 0:
		return fmt.Errorf("audio input requires an image or video reference")
	case firstFrame > 1 || lastFrame > 1:
		return fmt.Errorf("at most one first_frame and one last_frame are supported")
	}
	return nil
}

func videoString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
