package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

var ErrAssetCredentialProfileActive = errors.New("separate asset credential profile must be disabled before clearing its credential")

const VolcengineAssetActionRegion = "cn-beijing"

func GetChannelAssetCredentialStatus(channelID int, includeHint bool) (dto.ChannelAssetCredentialStatus, error) {
	credential, err := GetChannelAssetCredential(channelID)
	if err != nil || credential == nil {
		return dto.ChannelAssetCredentialStatus{}, err
	}
	status := dto.ChannelAssetCredentialStatus{Configured: true}
	if includeHint {
		status.AccessKeyIDHint = MaskAssetAccessKeyID(credential.AccessKeyID)
	}
	return status, nil
}

func MaskAssetAccessKeyID(value string) string {
	value = strings.TrimSpace(value)
	characters := []rune(value)
	if len(characters) <= 5 {
		return strings.Repeat("*", len(characters))
	}
	return string(characters[:2]) + "******" + string(characters[len(characters)-3:])
}

func NormalizeChannelAssetCredential(input *dto.ChannelAssetCredentialInput) (*ChannelAssetCredential, error) {
	if input == nil {
		return nil, nil
	}
	accessKeyID := strings.TrimSpace(input.AccessKeyID)
	secretAccessKey := strings.TrimSpace(input.SecretAccessKey)
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, errors.New("asset Access Key ID and Secret Access Key must both be provided")
	}
	if strings.Contains(accessKeyID, "|") {
		return nil, errors.New("asset Access Key ID must not contain the credential separator")
	}
	return &ChannelAssetCredential{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}, nil
}

func InsertChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
	return InsertChannelWithAssetCredentialActor(channel, input, 0)
}

func InsertChannelWithAssetCredentialActor(channel *Channel, input *dto.ChannelAssetCredentialInput, actorID int) error {
	credential, err := NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if channel == nil || credential == nil {
		return errors.New("channel and asset credential are required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		credential.ChannelID = channel.Id
		credential.CreatedTime = now
		credential.UpdatedTime = now
		if err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).Create(credential).Error; err != nil {
			return err
		}
		return channel.AddAbilitiesWithActor(tx, actorID)
	})
}

func UpdateChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
	return UpdateChannelWithAssetCredentialActor(channel, input, 0, false, false)
}

func UpdateChannelWithAssetCredentialActor(
	channel *Channel,
	input *dto.ChannelAssetCredentialInput,
	actorID int,
	assetTenantUnchanged bool,
	assetTenantReplacementConfirmed bool,
) error {
	credential, err := NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if channel == nil || credential == nil {
		return errors.New("channel and asset credential are required")
	}
	credential.ChannelID = channel.Id
	return updateChannelWithCredentialActor(
		channel,
		credential,
		actorID,
		assetTenantUnchanged,
		assetTenantReplacementConfirmed,
	)
}

// DeleteChannelAssetCredential refuses while the channel's published
// declaration still binds this asset protocol to the separate key pair slot.
// The declaration is read under the same publication lock as Channel writes
// (channel row lock first, then the plugin configuration lock), so a concurrent
// activation cannot flip the decision between check and commit. A missing or
// unreadable declaration fails closed: no valid declaration never means
// deletion permission.
func DeleteChannelAssetCredential(channelID int) error {
	if channelID <= 0 {
		return errors.New("channel ID is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		channel, err := lockChannelForMutation(tx, channelID)
		if err != nil {
			return err
		}
		if err := lockSeedancePluginConfiguration(tx, jsplugin.SeedancePluginKey); err != nil {
			return err
		}
		var active TaskPlugin
		err = lockForUpdate(tx).Where(&TaskPlugin{Key: jsplugin.SeedancePluginKey, Active: true}).First(&active).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && active.APIVersion < jsplugin.SeedanceConfigurationAPIVersion {
			return errors.New("Seedance plugin configuration is unavailable")
		}
		if err != nil {
			return err
		}
		configuration, err := seedanceConfigurationForArtifact(&active)
		if err != nil {
			return err
		}
		assetProtocol := string(channel.GetOtherSettings().AssetUpstreamProtocol)
		if assetProtocol == "" {
			assetProtocol = "none"
		}
		declaration := configuration.Asset(assetProtocol)
		if declaration == nil {
			return errors.New("Seedance plugin configuration is unavailable")
		}
		if declaration.Credential == "asset_key_pair" {
			return ErrAssetCredentialProfileActive
		}
		return deleteChannelAssetCredentialsTx(tx, []int{channelID})
	})
}

func saveChannelAssetCredentialTx(tx *gorm.DB, credential *ChannelAssetCredential) error {
	now := common.GetTimestamp()
	credential.UpdatedTime = now
	if credential.CreatedTime == 0 {
		credential.CreatedTime = now
	}
	return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"access_key_id":     credential.AccessKeyID,
			"secret_access_key": credential.SecretAccessKey,
			"updated_time":      credential.UpdatedTime,
		}),
	}).Create(credential).Error
}

func deleteChannelAssetCredentialsTx(tx *gorm.DB, channelIDs []int) error {
	if len(channelIDs) == 0 || !tx.Migrator().HasTable(&ChannelAssetCredential{}) {
		return nil
	}
	if err := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id IN ?", channelIDs).
		Delete(&ChannelAssetCredential{}).Error; err != nil {
		return fmt.Errorf("delete channel asset credentials: %w", err)
	}
	return nil
}
