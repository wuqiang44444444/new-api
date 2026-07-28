package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

// validateChannelVideoSettings validates and normalizes the protocol-specific
// settings used by Doubao video channels.
func validateChannelVideoSettings(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel.Status == common.ChannelStatusEnabled && channel.Type == constant.ChannelTypeSora {
		return fmt.Errorf("OpenAI video channels are retired")
	}
	if channel.Status == common.ChannelStatusEnabled {
		for _, modelName := range strings.Split(channel.Models, ",") {
			switch strings.TrimSpace(modelName) {
			case "sora-2", "sora-2-pro":
				return fmt.Errorf("OpenAI video model %q is retired", strings.TrimSpace(modelName))
			}
		}
	}
	if channel.Type != constant.ChannelTypeDoubaoVideo {
		return nil
	}
	if err := dto.ValidateVideoUpstreamProfile(settings.VideoUpstreamProfile); err != nil {
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
