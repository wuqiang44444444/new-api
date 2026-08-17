// Package feicai implements the Feicai Seedance video protocol.
package feicai

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const (
	BillingModePerSecond = "per-second"

	ProviderModelSeedance20Mini720P      = "seedance-2.0-vip-720p-mini-azhw"
	ProviderModelSeedance20SD2720P       = "seedance2.0-sd2"
	ProviderModelSeedance20Fast720P      = "seedance-2.0-vip-720p-fast-azhw"
	ProviderModelSeedance20Value720P     = "seedance-2.0-933-720p-azhw"
	ProviderModelSeedance20Standard720P  = "seedance-2.0-vip-720p-azhw"
	ProviderModelSeedance20Value1080P    = "seedance-2.0-933-1080p-azhw"
	ProviderModelSeedance20Standard1080P = "seedance-2.0-vip-1080p-azhw"
	ProviderModelSeedance20Value4K       = "seedance-2.0-933-4k-azhw"
	ProviderModelSeedance20Standard4K    = "seedance-2.0-vip-4k-azhw"
	ProviderModelSeedance20ProPI720P     = "seedance-933-pro-pi"

	resolution720P  = "720p"
	resolution1080P = "1080p"
	resolution4K    = "4k"
)

type ModelSpec struct {
	ProviderModel string
	Resolution    string
	MinDuration   int
	MaxDuration   int
	MinImages     int
	MaxImages     int
	MaxAudios     int
	MaxVideos     int
	Ratios        []string
}

var (
	standardRatios = []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"}
	sd2Ratios      = []string{"16:9", "9:16"}
)

var modelSpecs = []ModelSpec{
	{
		ProviderModel: ProviderModelSeedance20Mini720P, Resolution: resolution720P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20SD2720P, Resolution: resolution720P, MinDuration: 11, MaxDuration: 15,
		MinImages: 1, MaxImages: 9, Ratios: sd2Ratios,
	},
	{
		ProviderModel: ProviderModelSeedance20Fast720P, Resolution: resolution720P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Value720P, Resolution: resolution720P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Standard720P, Resolution: resolution720P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Value1080P, Resolution: resolution1080P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Standard1080P, Resolution: resolution1080P, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Value4K, Resolution: resolution4K, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20Standard4K, Resolution: resolution4K, MinDuration: 4, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, Ratios: standardRatios,
	},
	{
		ProviderModel: ProviderModelSeedance20ProPI720P, Resolution: resolution720P, MinDuration: 15, MaxDuration: 15,
		MaxImages: 9, MaxAudios: 3, MaxVideos: 3, Ratios: standardRatios,
	},
}

func CurrentModelSpecs() []ModelSpec {
	specs := slices.Clone(modelSpecs)
	for index := range specs {
		specs[index].Ratios = slices.Clone(specs[index].Ratios)
	}
	return specs
}

func ResolveModelSpec(providerModel string) (ModelSpec, bool) {
	providerModel = strings.TrimSpace(providerModel)
	for _, spec := range modelSpecs {
		if spec.ProviderModel != providerModel {
			continue
		}
		if len(spec.Ratios) == 0 {
			return ModelSpec{}, false
		}
		spec.Ratios = slices.Clone(spec.Ratios)
		return spec, true
	}
	return ModelSpec{}, false
}

type ResolvedRequest struct {
	Model    string
	Prompt   string
	Duration int
	Ratio    string
	Images   []string
	Audios   []string
	Videos   []string
	Spec     ModelSpec
}

func ResolveRequest(input *dto.ModelArkVideoCreateRequest, providerModel string) (ResolvedRequest, error) {
	if input == nil {
		return ResolvedRequest{}, fmt.Errorf("invalid video request")
	}
	providerModel = strings.TrimSpace(providerModel)
	spec, ok := ResolveModelSpec(providerModel)
	if !ok {
		return ResolvedRequest{}, fmt.Errorf("the selected customer model is not supported by its configured video adapter")
	}
	if input.Duration == nil || *input.Duration < spec.MinDuration || *input.Duration > spec.MaxDuration ||
		*input.Duration > relaycommon.MaxTaskDurationSeconds {
		return ResolvedRequest{}, fmt.Errorf("duration must be between %d and %d seconds for the selected customer model", spec.MinDuration, spec.MaxDuration)
	}
	if input.Resolution == nil || strings.ToLower(strings.TrimSpace(*input.Resolution)) != spec.Resolution {
		return ResolvedRequest{}, fmt.Errorf("resolution must be %q for the selected customer model", spec.Resolution)
	}
	if input.Ratio == nil {
		return ResolvedRequest{}, fmt.Errorf("aspect ratio is required for the selected customer model")
	}
	ratio := strings.TrimSpace(*input.Ratio)
	if !slices.Contains(spec.Ratios, ratio) {
		return ResolvedRequest{}, fmt.Errorf("aspect ratio %q is not supported by the selected customer model", ratio)
	}
	if err := rejectUnsupportedFields(input); err != nil {
		return ResolvedRequest{}, err
	}

	resolved := ResolvedRequest{Model: providerModel, Duration: *input.Duration, Ratio: ratio, Spec: spec}
	for _, item := range input.Content {
		switch strings.TrimSpace(item.Type) {
		case "text":
			text := ""
			if item.Text != nil {
				text = strings.TrimSpace(*item.Text)
			}
			if text != "" {
				if resolved.Prompt != "" {
					resolved.Prompt += "\n"
				}
				resolved.Prompt += text
			}
		case "image_url":
			if item.ImageURL == nil || item.Role == nil || strings.TrimSpace(*item.Role) != "reference_image" {
				return ResolvedRequest{}, fmt.Errorf("image_url requires role=reference_image")
			}
			mediaURL := strings.TrimSpace(item.ImageURL.URL)
			if err := validateMediaURL(mediaURL, mediaImage); err != nil {
				return ResolvedRequest{}, fmt.Errorf("invalid image input: %w", err)
			}
			resolved.Images = append(resolved.Images, mediaURL)
		case "audio_url":
			if item.AudioURL == nil || item.Role == nil || strings.TrimSpace(*item.Role) != "reference_audio" {
				return ResolvedRequest{}, fmt.Errorf("audio_url requires role=reference_audio")
			}
			mediaURL := strings.TrimSpace(item.AudioURL.URL)
			if err := validateMediaURL(mediaURL, mediaAudio); err != nil {
				return ResolvedRequest{}, fmt.Errorf("invalid audio input: %w", err)
			}
			resolved.Audios = append(resolved.Audios, mediaURL)
		case "video_url":
			if item.VideoURL == nil || item.Role == nil || strings.TrimSpace(*item.Role) != "reference_video" {
				return ResolvedRequest{}, fmt.Errorf("video_url requires role=reference_video")
			}
			mediaURL := strings.TrimSpace(item.VideoURL.URL)
			if err := validateMediaURL(mediaURL, mediaVideo); err != nil {
				return ResolvedRequest{}, fmt.Errorf("invalid video input: %w", err)
			}
			resolved.Videos = append(resolved.Videos, mediaURL)
		default:
			return ResolvedRequest{}, fmt.Errorf("unsupported content type %q", item.Type)
		}
	}
	if resolved.Prompt == "" {
		return ResolvedRequest{}, fmt.Errorf("prompt is required for the selected customer model")
	}
	if len(resolved.Images) < spec.MinImages || len(resolved.Images) > spec.MaxImages {
		return ResolvedRequest{}, fmt.Errorf("image count must be between %d and %d for the selected customer model", spec.MinImages, spec.MaxImages)
	}
	if len(resolved.Audios) > spec.MaxAudios {
		return ResolvedRequest{}, fmt.Errorf("audio count must not exceed %d for the selected customer model", spec.MaxAudios)
	}
	if len(resolved.Videos) > spec.MaxVideos {
		return ResolvedRequest{}, fmt.Errorf("video count must not exceed %d for the selected customer model", spec.MaxVideos)
	}
	return resolved, nil
}

func rejectUnsupportedFields(input *dto.ModelArkVideoCreateRequest) error {
	if input.CallbackURL != nil || input.ServiceTier != nil || input.GenerateAudio != nil || input.Watermark != nil ||
		input.ReturnLastFrame != nil || input.ExecutionExpiresAfter != nil || input.Draft != nil || input.Tools != nil ||
		input.SafetyIdentifier != nil || input.Priority != nil || input.Frames != nil || input.Seed != nil || input.CameraFixed != nil ||
		input.OutputFormat != nil {
		return fmt.Errorf("request contains fields unsupported by the selected customer model")
	}
	return nil
}
