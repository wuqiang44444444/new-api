package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
)

type ChannelAssetCredential struct {
	ChannelID       int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	AccessKeyID     string `json:"-" gorm:"type:text;not null"`
	SecretAccessKey string `json:"-" gorm:"type:text;not null"`
	CreatedTime     int64  `json:"-" gorm:"bigint"`
	UpdatedTime     int64  `json:"-" gorm:"bigint"`
}

var ErrAssetCredentialProfileActive = errors.New("official asset profile must be disabled before clearing its credential")

func GetChannelAssetCredential(channelID int) (*ChannelAssetCredential, error) {
	return getChannelAssetCredential(DB, channelID)
}

func getChannelAssetCredential(tx *gorm.DB, channelID int) (*ChannelAssetCredential, error) {
	var credential ChannelAssetCredential
	result := tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
		Where("channel_id = ?", channelID).
		Limit(1).
		Find(&credential)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &credential, nil
}

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

func OfficialAssetActionBaseURL(region string) string {
	return "https://ark." + strings.TrimSpace(region) + ".byteplusapi.com"
}

func OfficialAssetCredentialFingerprint(accessKeyID, secretAccessKey, project, region string) string {
	return AssetCredentialFingerprint(
		"official_action_assets/v2\n"+OfficialAssetActionBaseURL(region),
		strings.TrimSpace(accessKeyID)+"|"+strings.TrimSpace(secretAccessKey),
		string(dto.AssetUpstreamProfileOfficial),
		project,
		region,
	)
}

func ResolveAssetChannelCredential(channel *Channel) (string, string, error) {
	return resolveAssetChannelCredential(DB, channel, nil)
}

func resolveAssetChannelCredential(tx *gorm.DB, channel *Channel, override *ChannelAssetCredential) (string, string, error) {
	if channel == nil || channel.ChannelInfo.IsMultiKey {
		return "", "", errors.New("asset channel must use a single credential")
	}
	settings := channel.GetOtherSettings()
	if settings.AssetUpstreamProfile == dto.AssetUpstreamProfileOfficial {
		credential := override
		var err error
		if credential == nil {
			credential, err = getChannelAssetCredential(tx, channel.Id)
			if err != nil {
				return "", "", err
			}
		}
		if credential == nil || strings.TrimSpace(credential.AccessKeyID) == "" || strings.TrimSpace(credential.SecretAccessKey) == "" {
			return "", "", errors.New("official asset credential is not configured")
		}
		key := strings.TrimSpace(credential.AccessKeyID) + "|" + strings.TrimSpace(credential.SecretAccessKey)
		return key, OfficialAssetCredentialFingerprint(
			credential.AccessKeyID,
			credential.SecretAccessKey,
			settings.AssetProviderProject,
			settings.AssetRegion,
		), nil
	}
	keys := channel.GetKeys()
	if len(keys) != 1 || strings.TrimSpace(keys[0]) == "" {
		return "", "", errors.New("asset channel must contain exactly one credential")
	}
	key := strings.TrimSpace(keys[0])
	return key, AssetCredentialFingerprint(
		channel.GetBaseURL(),
		key,
		string(settings.AssetUpstreamProfile),
		settings.AssetProviderProject,
		settings.AssetRegion,
	), nil
}

func InsertChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
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
		if err := channel.AddAbilities(tx); err != nil {
			return err
		}
		now := common.GetTimestamp()
		credential.ChannelID = channel.Id
		credential.CreatedTime = now
		credential.UpdatedTime = now
		return tx.Session(&gorm.Session{Logger: tx.Logger.LogMode(gormlogger.Silent)}).
			Create(credential).Error
	})
}

func UpdateChannelWithAssetCredential(channel *Channel, input *dto.ChannelAssetCredentialInput) error {
	credential, err := NormalizeChannelAssetCredential(input)
	if err != nil {
		return err
	}
	if channel == nil || credential == nil {
		return errors.New("channel and asset credential are required")
	}
	credential.ChannelID = channel.Id
	return updateChannelWithAssetFence(channel, credential)
}

func DeleteChannelAssetCredential(channelID int) error {
	if channelID <= 0 {
		return errors.New("channel ID is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		channel, err := lockAssetLifecycleChannel(tx, channelID)
		if err != nil {
			return err
		}
		active, err := channelHasActiveAssetResourcesTx(tx, channelID)
		if err != nil {
			return err
		}
		if active {
			return fmt.Errorf("%w: channel %d", ErrChannelHasActiveAssetResources, channelID)
		}
		if channel.GetOtherSettings().AssetUpstreamProfile == dto.AssetUpstreamProfileOfficial {
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
