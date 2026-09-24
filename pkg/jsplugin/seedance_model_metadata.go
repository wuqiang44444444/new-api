package jsplugin

import (
	"fmt"
	"strings"
)

// SeedanceVideoModelMetadata describes the existing published ModelArk fields.
// It cannot create routes, customer models, prices or Provider-private fields.
type SeedanceVideoModelMetadata struct {
	AllowFrameImages            bool     `json:"allowFrameImages,omitempty"`
	MaxPromptLength             int      `json:"maxPromptLength,omitempty"`
	OmitDurationMaximum         bool     `json:"omitDurationMaximum,omitempty"`
	PublishGenerateAudioDefault bool     `json:"publishGenerateAudioDefault,omitempty"`
	AllowReturnLastFrame        bool     `json:"allowReturnLastFrame,omitempty"`
	AllowPriority               bool     `json:"allowPriority,omitempty"`
	DeleteVideo                 *bool    `json:"deleteVideo,omitempty"`
	DefaultDuration             int      `json:"defaultDuration,omitempty"`
	IntelligentDurationSeconds  int      `json:"intelligentDurationSeconds,omitempty"`
	DefaultGenerateAudio        bool     `json:"defaultGenerateAudio,omitempty"`
	AllowAudioOnly              bool     `json:"allowAudioOnly,omitempty"`
	MaxTotalMedia               int      `json:"maxTotalMedia,omitempty"`
	MinDuration                 int      `json:"minDuration,omitempty"`
	MaxDuration                 int      `json:"maxDuration,omitempty"`
	IntelligentDuration         bool     `json:"intelligentDuration,omitempty"`
	DurationRequired            bool     `json:"durationRequired,omitempty"`
	Resolutions                 []string `json:"resolutions,omitempty"`
	SuggestedResolutions        []string `json:"suggestedResolutions,omitempty"`
	ResolutionRequired          bool     `json:"resolutionRequired,omitempty"`
	FreeResolution              bool     `json:"freeResolution,omitempty"`
	Ratios                      []string `json:"ratios,omitempty"`
	RatioRequired               bool     `json:"ratioRequired,omitempty"`
	MaxImages                   int      `json:"maxImages,omitempty"`
	MaxVideos                   int      `json:"maxVideos,omitempty"`
	MinImages                   int      `json:"minImages,omitempty"`
	MaxAudios                   int      `json:"maxAudios,omitempty"`
	AllowVideos                 bool     `json:"allowVideos,omitempty"`
	AllowAudios                 bool     `json:"allowAudios,omitempty"`
	AllowGenerateAudio          bool     `json:"allowGenerateAudio,omitempty"`
	AllowWatermark              bool     `json:"allowWatermark,omitempty"`
	AllowSeed                   bool     `json:"allowSeed,omitempty"`
	AllowCameraFixed            bool     `json:"allowCameraFixed,omitempty"`
	OutputFormats               []string `json:"outputFormats,omitempty"`
	OmitOutputFormat            bool     `json:"omitOutputFormat,omitempty"`
	FullModelArk                bool     `json:"fullModelArk,omitempty"`
}

func (spec *SeedanceVideoModelMetadata) validate() error {
	if spec == nil {
		return nil
	}
	if spec.MaxPromptLength < 0 || spec.MaxPromptLength > 1000000 {
		return fmt.Errorf("invalid prompt length bound")
	}
	if spec.DefaultDuration < 0 || spec.DefaultDuration > 60 || spec.IntelligentDurationSeconds < 0 || spec.IntelligentDurationSeconds > 60 || spec.MaxTotalMedia < 0 || spec.MaxTotalMedia > 64 || spec.MinDuration < 0 || spec.MaxDuration < spec.MinDuration || spec.MaxDuration > 60 ||
		spec.MinImages < 0 || spec.MaxImages < 0 || spec.MaxVideos < 0 || spec.MaxAudios < 0 ||
		spec.MaxImages > 64 || spec.MaxVideos > 64 || spec.MaxAudios > 64 || (spec.MaxImages > 0 && spec.MinImages > spec.MaxImages) {
		return fmt.Errorf("invalid ModelArk metadata bounds")
	}
	if spec.DefaultDuration > 0 {
		if spec.MinDuration > 0 && spec.DefaultDuration < spec.MinDuration {
			return fmt.Errorf("default duration is below the declared minimum duration")
		}
		if spec.MaxDuration > 0 && spec.DefaultDuration > spec.MaxDuration {
			return fmt.Errorf("default duration is above the declared maximum duration")
		}
	}
	// Declared capabilities and published defaults must not contradict each
	// other: the public projection renders exactly these fields.
	if spec.PublishGenerateAudioDefault && spec.DefaultGenerateAudio && !spec.AllowGenerateAudio {
		return fmt.Errorf("published audio default requires declared audio capability")
	}
	if spec.MaxVideos > 0 && !spec.AllowVideos {
		return fmt.Errorf("video media limits require declared video capability")
	}
	if spec.MaxAudios > 0 && !spec.AllowAudios {
		return fmt.Errorf("audio media limits require declared audio capability")
	}
	if spec.ResolutionRequired && len(spec.Resolutions) == 0 {
		return fmt.Errorf("required resolution needs declared resolution options")
	}
	if spec.RatioRequired && len(spec.Ratios) == 0 {
		return fmt.Errorf("required ratio needs declared ratio options")
	}
	if spec.OmitOutputFormat && len(spec.OutputFormats) > 0 {
		return fmt.Errorf("unsupported output format cannot declare format options")
	}
	for _, values := range [][]string{spec.Resolutions, spec.SuggestedResolutions, spec.Ratios, spec.OutputFormats} {
		if len(values) > 32 {
			return fmt.Errorf("too many ModelArk metadata enum values")
		}
		seen := map[string]bool{}
		for _, value := range values {
			if value == "" || len(value) > 64 || strings.TrimSpace(value) != value || seen[value] {
				return fmt.Errorf("invalid ModelArk metadata enum")
			}
			seen[value] = true
		}
	}
	return nil
}
