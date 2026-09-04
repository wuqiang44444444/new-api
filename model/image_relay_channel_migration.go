package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

const (
	// legacyMoxingImageChannelType is the retired pre-image-relay Moxing slot.
	// The channel type renumber migration (see
	// seedance_channel_type_renumber_migration.go) moves historical Moxing rows
	// here before this migration collapses them into the async image type.
	legacyMoxingImageChannelType = 65
	imageRelayMigrationKey       = "migration.image_relay_protocol_v1"
	funCloudImageDefaultBaseURL  = "https://mm-internal-cn.leonecloud.com"
	moxingImageDefaultBaseURL    = "https://www.moxing.pro"
)

// migrateImageRelayChannels collapses the retired Moxing channel type into the
// ordinary image relay type and writes an explicit protocol on every affected
// channel. Runtime dispatch intentionally has no legacy type or inference path.
func migrateImageRelayChannels() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var marker Option
		err := tx.Where(&Option{Key: imageRelayMigrationKey}).First(&marker).Error
		if err == nil && marker.Value == "done" {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var channels []Channel
		if err := tx.Select("id", "type", "settings", "base_url", "param_override").
			Where("type IN ?", []int{constant.ChannelTypeAsyncImage, legacyMoxingImageChannelType}).
			Find(&channels).Error; err != nil {
			return err
		}
		for i := range channels {
			channel := &channels[i]
			if channel.ParamOverride != nil && strings.TrimSpace(*channel.ParamOverride) != "" {
				return fmt.Errorf("image relay migration requires channel %d parameter overrides to be removed", channel.Id)
			}

			settings := make(map[string]json.RawMessage)
			rawSettings := strings.TrimSpace(channel.OtherSettings)
			if rawSettings != "" {
				if err := common.UnmarshalJsonStr(rawSettings, &settings); err != nil {
					return fmt.Errorf("image relay migration cannot parse channel %d settings: %w", channel.Id, err)
				}
			}
			if settings == nil {
				settings = make(map[string]json.RawMessage)
			}

			protocol := dto.ImageUpstreamProtocol("")
			if rawProtocol, ok := settings["image_upstream_protocol"]; ok {
				if err := common.Unmarshal(rawProtocol, &protocol); err != nil {
					return fmt.Errorf("image relay migration cannot parse channel %d image protocol: %w", channel.Id, err)
				}
			}
			if channel.Type == legacyMoxingImageChannelType {
				if protocol != "" && protocol != dto.ImageUpstreamProtocolMoxingImagesV1 {
					return fmt.Errorf("legacy Moxing image channel %d has conflicting image protocol %q", channel.Id, protocol)
				}
				protocol = dto.ImageUpstreamProtocolMoxingImagesV1
			} else if protocol == "" {
				protocol = dto.ImageUpstreamProtocolFunCloudAIGCV2
			}
			if err := dto.ValidateImageUpstreamProtocol(protocol); err != nil {
				return fmt.Errorf("image relay channel %d: %w", channel.Id, err)
			}
			protocolJSON, err := common.Marshal(protocol)
			if err != nil {
				return err
			}
			settings["image_upstream_protocol"] = protocolJSON
			settingsJSON, err := common.Marshal(settings)
			if err != nil {
				return err
			}

			baseURL := ""
			if channel.BaseURL != nil {
				baseURL = strings.TrimSpace(*channel.BaseURL)
			}
			if baseURL == "" {
				baseURL = funCloudImageDefaultBaseURL
				if protocol == dto.ImageUpstreamProtocolMoxingImagesV1 {
					baseURL = moxingImageDefaultBaseURL
				}
			}
			result := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
				"type":     constant.ChannelTypeAsyncImage,
				"settings": string(settingsJSON),
				"base_url": baseURL,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("image relay migration expected one channel update for id=%d, got %d", channel.Id, result.RowsAffected)
			}
		}
		return tx.Save(&Option{Key: imageRelayMigrationKey, Value: "done"}).Error
	})
}

// verifyImageRelayMigrationState is a read-only startup gate used by every
// node. It prevents a new runtime from serving traffic before the master has
// retired type 63 and materialized an explicit protocol on every image relay.
func verifyImageRelayMigrationState() error {
	var marker Option
	if err := DB.Where(&Option{Key: imageRelayMigrationKey}).First(&marker).Error; err != nil {
		return fmt.Errorf("image relay migration is not complete: %w", err)
	}
	if marker.Value != "done" {
		return fmt.Errorf("image relay migration is not complete")
	}

	var channels []Channel
	if err := DB.Select("id", "type", "settings", "base_url", "param_override").
		Where("type IN ?", []int{constant.ChannelTypeAsyncImage, legacyMoxingImageChannelType}).
		Find(&channels).Error; err != nil {
		return err
	}
	for i := range channels {
		channel := &channels[i]
		if channel.Type == legacyMoxingImageChannelType {
			return fmt.Errorf("image relay migration verification found legacy channel %d", channel.Id)
		}
		if channel.ParamOverride != nil && strings.TrimSpace(*channel.ParamOverride) != "" {
			return fmt.Errorf("image relay migration verification found parameter overrides on channel %d", channel.Id)
		}
		if channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "" {
			return fmt.Errorf("image relay migration verification found missing base URL on channel %d", channel.Id)
		}
		settings := dto.ChannelOtherSettings{}
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return fmt.Errorf("image relay migration verification cannot parse channel %d settings: %w", channel.Id, err)
		}
		if err := dto.ValidateImageUpstreamProtocol(settings.ImageUpstreamProtocol); err != nil {
			return fmt.Errorf("image relay migration verification failed for channel %d: %w", channel.Id, err)
		}
	}
	return nil
}
