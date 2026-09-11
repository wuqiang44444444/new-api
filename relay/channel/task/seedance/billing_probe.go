package seedance

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

const modelArkIntelligentDurationBillingSeconds = 15

// BuildTaskBillingProbe derives expression inputs from the same typed payload
// sent upstream. A video input only counts when its URL is non-empty, so an
// empty placeholder cannot select a cheaper pricing tier.
func (a *TaskAdaptor) BuildTaskBillingProbe(c *gin.Context, info *common.RelayInfo) (map[string]any, error) {
	if info == nil {
		return nil, fmt.Errorf("relay info is unavailable")
	}
	payload, typed, err := a.modelArkContractPayload(c)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload for billing probe failed")
	}
	if !typed {
		return nil, fmt.Errorf("Seedance billing requires the ModelArk V3 request contract")
	}

	resolution := strings.ToLower(strings.TrimSpace(payload.Resolution))
	if resolution == "" {
		resolution = "720p"
	}
	providerModel := providerModelFromRelayInfo(info, payload.Model)
	if SeedanceExtensionProtocolMigrated(a.protocol) && info.ChannelMeta == nil {
		return nil, fmt.Errorf("billing capability is unavailable for the selected customer model")
	}
	spec, hasSpec, specErr := a.pinnedProviderSpec(c, providerModel)
	if specErr != nil {
		return nil, specErr
	}
	if hasSpec && (a.protocol == dto.VideoUpstreamProtocolMoxingModelArkV1 || a.protocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 || a.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 || !SeedanceExtensionProtocolMigrated(a.protocol)) {
		// Registry-driven models may publish no resolution enum; unlisted
		// values then stay a provider decision and are recorded as-is.
		if len(spec.resolutions) > 0 {
			if _, allowed := spec.resolutions[resolution]; !allowed {
				return nil, fmt.Errorf("resolution %q is not supported by the selected customer model", resolution)
			}
		}
	} else {
		switch resolution {
		case "480p", "720p", "1080p", "4k":
		default:
			return nil, fmt.Errorf("resolution must be one of 480p, 720p, 1080p, or 4k")
		}
	}

	hasVideoInput := false
	for _, item := range payload.Content {
		if item.Type == "video_url" && item.VideoURL != nil && strings.TrimSpace(item.VideoURL.URL) != "" {
			hasVideoInput = true
			break
		}
	}
	durationSeconds := 5
	if hasSpec && spec.defaultDuration > 0 {
		durationSeconds = spec.defaultDuration
	}
	if payload.Duration != nil {
		durationSeconds = int(*payload.Duration)
	}
	if payload.Frames != nil {
		durationSeconds = (int(*payload.Frames) + 23) / 24
	}
	// ModelArk uses -1 for intelligent duration. Pre-consume against the
	// provider's maximum possible duration. A successful terminal response with
	// actual usage may later settle below this frozen upper bound.
	if durationSeconds == -1 {
		durationSeconds = modelArkIntelligentDurationBillingSeconds
		if hasSpec && spec.intelligentDuration > 0 {
			durationSeconds = spec.intelligentDuration
		}
	}
	if durationSeconds < 0 || durationSeconds > common.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("duration_seconds must be between 0 and %d", common.MaxTaskDurationSeconds)
	}
	generateAudio := false
	if hasSpec {
		generateAudio = spec.defaultGenerateAudio
	}
	if payload.GenerateAudio != nil {
		generateAudio = bool(*payload.GenerateAudio)
	}
	inputMode, controlMode := relayBillingModes(payload)
	if a.protocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 {
		inputMode, controlMode = tokenSaveBillingModes(payload)
	}

	probe := map[string]any{
		"resolution":       resolution,
		"has_video_input":  hasVideoInput,
		"duration_seconds": durationSeconds,
		"generate_audio":   generateAudio,
		"input_mode":       inputMode,
		"control_mode":     controlMode,
	}
	if SeedanceExtensionProtocolMigrated(a.protocol) {
		if info.ChannelMeta == nil {
			return nil, fmt.Errorf("billing capability is unavailable for the selected customer model")
		}
		contract, contractOK := common.GetVideoContractRequest(c)
		if !contractOK || contract.ModelArk == nil {
			return nil, fmt.Errorf("billing requires the ModelArk V3 request contract")
		}
		// feicai probe 字段（resolution/ratio/size_multiplier/billing_mode）由
		// seedance-link 插件的 buildCreate 转换结果导出；转换错误保持与旧
		// Go ResolveRequest 相同的消息与失败路径。
		conversion, conversionErr := a.ensureSeedanceCreateConversion(c, info)
		if conversionErr != nil {
			return nil, conversionErr
		}
		for key, value := range conversion.probe {
			probe[key] = value
		}
	}
	if a.protocol == dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		probe["billing_mode"] = "per-second"
		if payload.Duration != nil && *payload.Duration == -1 {
			probe["billing_mode"] = "per-token"
		}
	}
	return probe, nil
}
