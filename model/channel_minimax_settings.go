package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

// validateMinimaxChannelSettings validates MiniMax Link channel settings on
// management writes: one credential, the code-registered protocol, no asset
// library, and an absolute HTTP(S) base URL that can serve the fixed JD
// Cloud task paths.
func validateMinimaxChannelSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	return validateMinimaxChannelSettingsTx(DB, channel, settings)
}

func validateMinimaxChannelSettingsTx(tx *gorm.DB, channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || channel.Type != constant.ChannelTypeMiniMaxLink {
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
		return fmt.Errorf("MiniMax Link channels require one channel credential")
	}
	if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1) {
		return fmt.Errorf("unsupported MiniMax Link video upstream protocol %q", settings.VideoUpstreamProtocol)
	}
	settings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone
	settings.VideoUpstreamProfile = ""
	settings.VideoUpstreamCreatePath = ""
	settings.VideoUpstreamQueryPathTemplate = ""
	if err := dto.ValidateVideoUpstreamURL(channel.GetBaseURL(), constant.JdCloudCreatePath, constant.JdCloudQueryPathTemplate); err != nil {
		return err
	}
	normalized, err := common.Marshal(settings)
	if err != nil {
		return err
	}
	channel.OtherSettings = string(normalized)
	return nil
}
