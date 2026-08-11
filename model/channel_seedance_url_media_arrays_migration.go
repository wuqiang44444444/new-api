package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
)

const legacySeedanceVideoProtocolMediaArraysV2 = "media_arrays_v2"

// migrateSeedanceURLMediaArraysProtocol replaces the retired administrator-facing
// protocol identity on current channels. Historical task and attempt snapshots
// remain unchanged because they describe the transport used at creation time.
func migrateSeedanceURLMediaArraysProtocol() error {
	if !DB.Migrator().HasTable(&Channel{}) {
		return nil
	}

	var channels []Channel
	if err := DB.Select("id", "settings").
		Where("type = ?", constant.ChannelTypeSeedanceLink).
		Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		channel := &channels[i]
		if !strings.Contains(channel.OtherSettings, legacySeedanceVideoProtocolMediaArraysV2) {
			continue
		}
		var settings dto.ChannelOtherSettings
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return fmt.Errorf("decode Seedance channel %d settings for URL media protocol migration: %w", channel.Id, err)
		}
		if string(settings.VideoUpstreamProtocol) != legacySeedanceVideoProtocolMediaArraysV2 {
			continue
		}
		settings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolURLMediaArraysV1
		encoded, err := common.Marshal(settings)
		if err != nil {
			return fmt.Errorf("encode Seedance channel %d settings for URL media protocol migration: %w", channel.Id, err)
		}
		if err := DB.Model(&Channel{}).
			Where("id = ? AND settings = ?", channel.Id, channel.OtherSettings).
			Update("settings", string(encoded)).Error; err != nil {
			return fmt.Errorf("migrate Seedance channel %d URL media protocol: %w", channel.Id, err)
		}
	}
	return nil
}
