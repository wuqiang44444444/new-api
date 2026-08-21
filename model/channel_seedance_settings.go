package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

var officialAssetRegionPattern = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]+$`)

func validateSeedanceChannelSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	credentialChannel := *channel
	isMultiKey := channel.ChannelInfo.IsMultiKey
	if channel.Id > 0 {
		var persisted Channel
		if err := DB.Select("id", "key", "channel_info").First(&persisted, channel.Id).Error; err == nil {
			isMultiKey = isMultiKey || persisted.ChannelInfo.IsMultiKey
			if strings.TrimSpace(credentialChannel.Key) == "" {
				credentialChannel.Key = persisted.Key
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if isMultiKey || len(credentialChannel.GetKeys()) != 1 {
		return fmt.Errorf("Seedance Link channels require one channel credential")
	}
	if err := dto.ValidateVideoUpstreamProtocol(settings.VideoUpstreamProtocol); err != nil {
		return err
	}
	if err := validateMoxingTokenSaveModelMapping(channel, settings.VideoUpstreamProtocol); err != nil {
		return err
	}
	if settings.AssetUpstreamProtocol == "" {
		settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
	}
	if err := dto.ValidateAssetUpstreamProtocol(settings.AssetUpstreamProtocol); err != nil {
		return err
	}
	if err := validateFunCloudSeedanceChannel(channel, settings); err != nil {
		return err
	}
	if err := validateCMCCSeedanceChannel(channel, settings); err != nil {
		return err
	}

	settings.VideoUpstreamProfile = ""
	settings.AssetUpstreamProfile = ""

	settings.VideoUpstreamCreatePath = ""
	settings.VideoUpstreamQueryPathTemplate = ""
	if !settings.VideoUpstreamProtocol.TransportProfile().IsOfficial() {
		if err := dto.ValidateVideoUpstreamURL(channel.GetBaseURL(), "/create", "/tasks/{task_id}"); err != nil {
			return err
		}
	}
	if settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolNone && settings.AssetMinURLTTLSeconds <= 0 {
		return fmt.Errorf("Seedance asset protocol requires a positive remote URL minimum TTL")
	}
	switch settings.AssetUpstreamProtocol {
	case dto.AssetUpstreamProtocolVolcengineAction:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3Volcengine {
			return fmt.Errorf("Volcengine asset protocol requires the Volcengine ModelArk V3 video protocol")
		}
		if strings.TrimSpace(settings.AssetProviderProject) == "" {
			return fmt.Errorf("official Seedance asset protocol requires ProviderProject")
		}
		if strings.TrimSpace(settings.AssetRegion) != VolcengineAssetActionRegion {
			return fmt.Errorf("Volcengine asset protocol Region must be %s", VolcengineAssetActionRegion)
		}
	case dto.AssetUpstreamProtocolBytePlusAction:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3BytePlus {
			return fmt.Errorf("BytePlus asset protocol requires the BytePlus ModelArk V3 video protocol")
		}
		if strings.TrimSpace(settings.AssetProviderProject) == "" || strings.TrimSpace(settings.AssetRegion) == "" {
			return fmt.Errorf("official Seedance asset protocol requires ProviderProject and Region")
		}
		if !officialAssetRegionPattern.MatchString(strings.TrimSpace(settings.AssetRegion)) {
			return fmt.Errorf("official Seedance asset protocol Region is invalid")
		}
	case dto.AssetUpstreamProtocolArkAssetsV1:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolArkMediaV1 {
			return fmt.Errorf("Ark asset protocol requires the Ark Media V1 video protocol")
		}
	case dto.AssetUpstreamProtocolTokenSaveAssetsV1:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 {
			return fmt.Errorf("TokenSave asset protocol requires the TokenSave Media Task V1 video protocol")
		}
	case dto.AssetUpstreamProtocolMoxingJoyCreatorV1:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolMoxingMediaTaskV1 {
			return fmt.Errorf("Moxing JoyCreator asset protocol requires the Moxing Media Task V1 video protocol")
		}
	case dto.AssetUpstreamProtocolMoxingVolcAssetsV1:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolMoxingModelArkV1 {
			return fmt.Errorf("Moxing Volcengine asset protocol requires the Moxing ModelArk Media V1 video protocol")
		}
		settings.AssetProviderProject = "default"
	case dto.AssetUpstreamProtocolFunCloudMaterial:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolFunCloudSeedance {
			return fmt.Errorf("FunCloud material protocol requires the FunCloud Seedance video protocol")
		}
	case dto.AssetUpstreamProtocolCMCCAICCV2:
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolModelArkV3CMCC {
			return fmt.Errorf("CMCC AICC assets require the CMCC ModelArk V3 video protocol")
		}
	}

	normalized, err := common.Marshal(settings)
	if err != nil {
		return err
	}
	channel.OtherSettings = string(normalized)
	return nil
}

func validateMoxingTokenSaveModelMapping(channel *Channel, protocol dto.VideoUpstreamProtocol) error {
	providerModels := map[string]struct{}{}
	switch protocol {
	case dto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
		providerModels["doubao-seedance-2-0-260128"] = struct{}{}
	case dto.VideoUpstreamProtocolMoxingMediaTaskV1:
		providerModels["doubao-seedance-2-0-260128"] = struct{}{}
	case dto.VideoUpstreamProtocolMoxingModelArkV1:
		providerModels["doubao-seedance-2-0-fast-260128"] = struct{}{}
		providerModels["doubao-seedance-2-0-mini-260615"] = struct{}{}
		providerModels["doubao-seedance-2-5-260628"] = struct{}{}
	default:
		return nil
	}

	models := channel.GetModels()
	if len(models) != 1 {
		return fmt.Errorf("TokenSave and Moxing Seedance channels require exactly one customer model")
	}
	customerModel := strings.TrimSpace(models[0])
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping); err != nil {
		return fmt.Errorf("TokenSave and Moxing Seedance channels require one exact model_mapping entry")
	}
	providerModel := strings.TrimSpace(mapping[customerModel])
	if len(mapping) != 1 || providerModel == "" {
		return fmt.Errorf("model_mapping must contain exactly one entry for customer model %q", customerModel)
	}
	if _, supported := providerModels[providerModel]; !supported {
		return fmt.Errorf("model_mapping target for %q is not supported by video protocol %s", customerModel, protocol)
	}
	return nil
}

// ValidateSeedanceChannelModelUniqueness enforces the administrator-facing
// one-model/one-channel rule only on management writes. Runtime routing does
// not repeat this audit or repair invalid direct database edits.
func ValidateSeedanceChannelModelUniqueness(tx *gorm.DB, channel *Channel) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink || channel.Status != common.ChannelStatusEnabled {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	models := make([]string, 0)
	seen := make(map[string]struct{})
	for _, value := range channel.GetModels() {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		models = append(models, value)
	}
	if len(models) == 0 {
		return fmt.Errorf("Seedance Link channel requires at least one customer model")
	}
	var channels []Channel
	query := tx.Where("type = ? AND status = ?", constant.ChannelTypeSeedanceLink, common.ChannelStatusEnabled)
	if channel.Id > 0 {
		query = query.Where("id <> ?", channel.Id)
	}
	if err := query.Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		for _, modelName := range models {
			if channelContainsModel(&channels[i], modelName) {
				return fmt.Errorf("Seedance model %q is already enabled on channel %q (#%d). Disable it there before enabling this channel", modelName, channels[i].Name, channels[i].Id)
			}
		}
	}
	return nil
}
