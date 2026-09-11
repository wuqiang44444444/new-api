package seedance

import (
	"fmt"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"strings"
)

func providerSpec(protocol kitdto.VideoUpstreamProtocol, model string) (providerModelSpec, bool) {
	model = strings.TrimSpace(model)
	switch protocol {
	case kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
		if model != modelSeedance20 {
			return providerModelSpec{}, false
		}
		return providerModelSpec{minDuration: 4, maxDuration: 15, intelligentDuration: 15, allowIntelligentDuration: true, resolutions: stringSet("480p", "720p", "1080p"), allowVideos: true, allowAudios: true, allowAudioOnly: true}, true
	case kitdto.VideoUpstreamProtocolMoxingModelArkV1:
		if contract, ok := kitdto.MoxingVideoModelContractFor(model); ok {
			return moxingProviderSpec(contract), true
		}
	case kitdto.VideoUpstreamProtocolFunCloudModelArkV3:
		return funCloudModelArkProviderSpec(model)
	}
	return providerModelSpec{}, false
}

// moxingProviderSpec derives the runtime rules from the single Moxing model
// registry. It adds no per-model restriction beyond the registered contract:
// undocumented caps (reference media quantities for 2.0/Fast/Mini and the
// resolution enum) stay unset so the provider judges unlisted inputs.
func moxingProviderSpec(contract kitdto.MoxingVideoModelContract) providerModelSpec {
	return providerModelSpec{
		minDuration:              contract.MinDurationSeconds,
		maxDuration:              contract.MaxDurationSeconds,
		intelligentDuration:      contract.IntelligentDurationSeconds,
		allowIntelligentDuration: contract.IntelligentDurationSeconds > 0,
		maxImages:                contract.MaxImages,
		maxVideos:                contract.MaxVideos,
		maxAudios:                contract.MaxAudios,
		maxTotalMedia:            contract.MaxTotalMedia,
		allowVideos:              contract.AllowReferenceVideos,
		allowAudios:              contract.AllowReferenceAudios,
		allowAudioOnly:           contract.AllowAudioOnly,
		defaultGenerateAudio:     contract.DefaultGenerateAudio,
		// These are the published northbound formats, not inferred per-model capabilities.
		outputFormats: stringSet("mp4", "mov"),
	}
}

func validateProviderModelRequest(protocol kitdto.VideoUpstreamProtocol, model string, request *dto.ModelArkVideoCreateRequest) error {
	spec, ok := providerSpec(protocol, model)
	if !ok {
		return fmt.Errorf("the selected customer model is not supported by its configured video adapter")
	}
	if request == nil {
		return fmt.Errorf("ModelArk request is required")
	}
	// Moxing forwards the published ModelArk fields. Missing entries in a
	// provider parameter table do not establish an unsupported-field rule.
	if protocol != kitdto.VideoUpstreamProtocolMoxingModelArkV1 {
		if protocol != kitdto.VideoUpstreamProtocolFunCloudModelArkV3 && (request.ReturnLastFrame != nil || request.Priority != nil) {
			return fmt.Errorf("request contains a parameter unsupported by the selected customer model")
		}
		if request.CallbackURL != nil || request.ServiceTier != nil ||
			request.ExecutionExpiresAfter != nil || request.Draft != nil || request.Tools != nil ||
			request.SafetyIdentifier != nil || request.Frames != nil {
			return fmt.Errorf("request contains a parameter unsupported by the selected customer model")
		}
	}
	duration := 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	if duration == -1 && !spec.allowIntelligentDuration {
		return fmt.Errorf("intelligent duration is not supported by the selected customer model")
	}
	if duration != -1 && (duration < spec.minDuration || duration > spec.maxDuration) {
		return fmt.Errorf("duration must be between %d and %d for the selected customer model", spec.minDuration, spec.maxDuration)
	}
	resolution := "720p"
	if request.Resolution != nil {
		resolution = strings.TrimSpace(*request.Resolution)
		if resolution == "" {
			return fmt.Errorf("resolution must not be empty")
		}
	}
	// A model with no registered resolution enum (the Moxing models) leaves
	// unlisted values to the provider instead of guessing its spec locally.
	if len(spec.resolutions) > 0 {
		if _, allowed := spec.resolutions[resolution]; !allowed {
			return fmt.Errorf("resolution %q is not supported by the selected customer model", resolution)
		}
	}
	if request.Ratio != nil {
		ratio := strings.TrimSpace(*request.Ratio)
		if ratio == "" {
			return fmt.Errorf("ratio must not be empty")
		}
		if _, allowed := modelArkRatios[ratio]; !allowed {
			return fmt.Errorf("ratio %q is not supported by the selected customer model", ratio)
		}
	}
	format := ""
	if request.OutputFormat != nil {
		format = strings.TrimSpace(*request.OutputFormat)
		if format == "" {
			return fmt.Errorf("output_format must not be empty")
		}
	}
	if format != "" {
		if _, allowed := spec.outputFormats[format]; !allowed {
			return fmt.Errorf("output_format %q is not supported by the selected customer model", format)
		}
	}

	images, videos, audios := 0, 0, 0
	for _, item := range request.Content {
		switch item.Type {
		case "image_url":
			images++
		case "video_url":
			videos++
		case "audio_url":
			audios++
		}
	}
	if videos > 0 && !spec.allowVideos {
		return fmt.Errorf("video_url content is not supported by the selected customer model")
	}
	if audios > 0 && !spec.allowAudios {
		return fmt.Errorf("audio_url content is not supported by the selected customer model")
	}
	if spec.maxImages > 0 && images > spec.maxImages {
		return fmt.Errorf("at most %d reference images are supported by the selected customer model", spec.maxImages)
	}
	if spec.maxVideos > 0 && videos > spec.maxVideos {
		return fmt.Errorf("at most %d reference videos are supported by the selected customer model", spec.maxVideos)
	}
	if spec.maxAudios > 0 && audios > spec.maxAudios {
		return fmt.Errorf("at most %d reference audios are supported by the selected customer model", spec.maxAudios)
	}
	if spec.maxTotalMedia > 0 && images+videos+audios > spec.maxTotalMedia {
		return fmt.Errorf("at most %d reference media items are supported by the selected customer model", spec.maxTotalMedia)
	}
	if !spec.allowAudioOnly && audios > 0 && images == 0 && videos == 0 {
		return fmt.Errorf("audio-only input is not supported by the selected customer model")
	}
	return nil
}

var modelArkRatios = stringSet("16:9", "4:3", "1:1", "3:4", "9:16", "21:9", "adaptive")

func providerBillingDefaults(protocol kitdto.VideoUpstreamProtocol, model string) (int, bool, bool) {
	spec, ok := providerSpec(protocol, model)
	if !ok {
		return 0, false, false
	}
	return spec.intelligentDuration, spec.defaultGenerateAudio, true
}
