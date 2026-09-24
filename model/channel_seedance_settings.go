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
	// The ModelArk V3 standard entry is shared by the typed video channels,
	// so the shared uniqueness contract dispatches by type from this gate.
	if channel.Type == constant.ChannelTypeMiniMaxLink {
		return ValidateMiniMaxChannelModelUniqueness(tx, channel)
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
	return validateTypedStandardVideoModelConflict(tx, channel, "Seedance Link")
}
