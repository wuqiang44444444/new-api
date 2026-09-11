package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

func validateSeedanceChannelSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	return validateSeedanceChannelSettingsTx(DB, channel, settings)
}

func validateSeedanceChannelSettingsTx(tx *gorm.DB, channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || channel.Type != constant.ChannelTypeSeedanceLink {
		return nil
	}
	if tx == nil {
		tx = DB
	}
	credentialChannel := *channel
	isMultiKey := channel.ChannelInfo.IsMultiKey
	if channel.Id > 0 {
		var persisted Channel
		if err := tx.Select("id", "key", "channel_info").First(&persisted, channel.Id).Error; err == nil {
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
	if settings.AssetUpstreamProtocol == "" {
		settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
	}
	if err := dto.ValidateAssetUpstreamProtocol(settings.AssetUpstreamProtocol); err != nil {
		return err
	}

	settings.VideoUpstreamProfile = ""
	settings.AssetUpstreamProfile = ""

	settings.VideoUpstreamCreatePath = ""
	settings.VideoUpstreamQueryPathTemplate = ""
	if channel.GetBaseURL() != "" || !settings.VideoUpstreamProtocol.TransportProfile().IsOfficial() || settings.VideoUpstreamProtocol == dto.VideoUpstreamProtocolModelArkV3CMCC {
		if err := dto.ValidateVideoUpstreamURL(channel.GetBaseURL(), "/create", "/tasks/{task_id}"); err != nil {
			return err
		}
	}
	// The hosted FunCloud path copies the source into platform storage at
	// creation time, so no Provider fetch window has to be guaranteed.
	if settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolNone &&
		settings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolFunCloudHosted &&
		settings.AssetMinURLTTLSeconds <= 0 {
		return fmt.Errorf("Seedance asset protocol requires a positive remote URL minimum TTL")
	}

	normalized, err := common.Marshal(settings)
	if err != nil {
		return err
	}
	channel.OtherSettings = string(normalized)
	return nil
}

// ValidateSeedanceChannelModelUniqueness keeps Link/native price keys distinct
// and enforces one enabled Seedance channel per model on management writes.
// Runtime routing does not repeat this audit or repair direct database edits.
func ValidateSeedanceChannelModelUniqueness(tx *gorm.DB, channel *Channel) error {
	if channel == nil {
		return nil
	}
	if err := validateSeedancePublishedChannelConfiguration(tx, channel); err != nil {
		return err
	}
	if err := validateSeedancePricingOwnership(tx, channel); err != nil {
		return err
	}
	if channel.Type != constant.ChannelTypeSeedanceLink || channel.Status != common.ChannelStatusEnabled {
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
