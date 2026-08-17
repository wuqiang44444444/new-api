package seedance

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

const (
	modelSeedance20     = "doubao-seedance-2-0-260128"
	modelSeedance20Fast = "doubao-seedance-2-0-fast-260128"
	modelSeedance20Mini = "doubao-seedance-2-0-mini-260615"
	modelSeedance25     = "doubao-seedance-2-5-260628"
)

type providerModelSpec struct {
	minDuration              int
	maxDuration              int
	intelligentDuration      int
	allowIntelligentDuration bool
	resolutions              map[string]struct{}
	maxImages                int
	maxVideos                int
	maxAudios                int
	allowVideos              bool
	allowAudios              bool
	allowAudioOnly           bool
	defaultGenerateAudio     bool
	outputFormats            map[string]struct{}
}

func providerSpec(protocol kitdto.VideoUpstreamProtocol, model string) (providerModelSpec, bool) {
	model = strings.TrimSpace(model)
	switch protocol {
	case kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
		if model != modelSeedance20 {
			return providerModelSpec{}, false
		}
		return providerModelSpec{minDuration: 4, maxDuration: 15, intelligentDuration: 15, allowIntelligentDuration: true, resolutions: stringSet("480p", "720p", "1080p")}, true
	case kitdto.VideoUpstreamProtocolMoxingMediaTaskV1:
		if model != modelSeedance20 {
			return providerModelSpec{}, false
		}
		return providerModelSpec{
			minDuration: 4, maxDuration: 15, intelligentDuration: 15, allowIntelligentDuration: true,
			resolutions: stringSet("480p", "720p"), allowVideos: true, allowAudios: true,
		}, true
	case kitdto.VideoUpstreamProtocolMoxingModelArkV1:
		switch model {
		case modelSeedance20Fast, modelSeedance20Mini:
			return providerModelSpec{
				minDuration: 4, maxDuration: 15, intelligentDuration: 15, allowIntelligentDuration: true,
				resolutions: stringSet("480p", "720p"), maxImages: 9, maxVideos: 3, maxAudios: 3,
				allowVideos: true, allowAudios: true, defaultGenerateAudio: true,
			}, true
		case modelSeedance25:
			return providerModelSpec{
				minDuration: 4, maxDuration: 30, intelligentDuration: 30, allowIntelligentDuration: true,
				resolutions: stringSet("480p", "720p"), maxImages: 30, maxVideos: 10, maxAudios: 10,
				allowVideos: true, allowAudios: true, allowAudioOnly: true,
				defaultGenerateAudio: true, outputFormats: stringSet("mp4", "mov"),
			}, true
		}
	case kitdto.VideoUpstreamProtocolFunCloudSeedance:
		return funCloudProviderSpec(model)
	}
	return providerModelSpec{}, false
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validateProviderModelRequest(protocol kitdto.VideoUpstreamProtocol, model string, request *dto.ModelArkVideoCreateRequest) error {
	spec, ok := providerSpec(protocol, model)
	if !ok {
		return fmt.Errorf("the selected customer model is not supported by its configured video adapter")
	}
	if request == nil {
		return fmt.Errorf("ModelArk request is required")
	}
	switch protocol {
	case kitdto.VideoUpstreamProtocolMoxingMediaTaskV1,
		kitdto.VideoUpstreamProtocolMoxingModelArkV1:
		if request.CameraFixed != nil {
			return fmt.Errorf("camera_fixed is not supported by the selected model")
		}
		if request.Seed != nil && model != modelSeedance25 {
			return fmt.Errorf("seed is not supported by the selected model")
		}
	}
	if request.CallbackURL != nil || request.ReturnLastFrame != nil || request.ServiceTier != nil ||
		request.ExecutionExpiresAfter != nil || request.Draft != nil || request.Tools != nil ||
		request.SafetyIdentifier != nil || request.Priority != nil || request.Frames != nil {
		return fmt.Errorf("request contains a parameter unsupported by the selected customer model")
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
	if _, allowed := spec.resolutions[resolution]; !allowed {
		return fmt.Errorf("resolution %q is not supported by the selected customer model", resolution)
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

func providerModelFromRelayInfo(info *relaycommon.RelayInfo, requestModel string) string {
	if info != nil && info.ChannelMeta != nil && strings.TrimSpace(info.UpstreamModelName) != "" {
		return strings.TrimSpace(info.UpstreamModelName)
	}
	return strings.TrimSpace(requestModel)
}
