package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

// validateChannelVideoSettings validates and normalizes the protocol-specific
// settings used by Doubao video channels.
func validateChannelVideoSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel.Type != constant.ChannelTypeDoubaoVideo {
		return nil
	}
	if err := dto.ValidateVideoUpstreamProfile(settings.VideoUpstreamProfile); err != nil {
		return err
	}
	if err := validateJSONVideoMediaArraysChannel(channel, settings); err != nil {
		return err
	}
	if err := validateFunCloudVideoProfileChannel(channel, settings); err != nil {
		return err
	}
	if settings.VideoUpstreamProfile.IsOfficial() {
		if settings.VideoUpstreamCreatePath == "" && settings.VideoUpstreamQueryPathTemplate == "" {
			return nil
		}
		settings.VideoUpstreamCreatePath = ""
		settings.VideoUpstreamQueryPathTemplate = ""
		normalized, err := common.Marshal(settings)
		if err != nil {
			return err
		}
		channel.OtherSettings = string(normalized)
		return nil
	}

	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = *channel.BaseURL
	}
	return dto.ValidateVideoUpstreamURL(
		baseURL,
		settings.VideoUpstreamCreatePath,
		settings.VideoUpstreamQueryPathTemplate,
	)
}
